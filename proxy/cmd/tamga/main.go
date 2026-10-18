package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/yatuk/tamga/internal/analyzer"
	"github.com/yatuk/tamga/internal/api"
	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/billing"
	"github.com/yatuk/tamga/internal/budget"
	"github.com/yatuk/tamga/internal/buildinfo"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/docstore"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/grpcauth"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/patterns"
	"github.com/yatuk/tamga/internal/policy"
	policyhistory "github.com/yatuk/tamga/internal/policy/history"
	"github.com/yatuk/tamga/internal/proxy"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/redisx"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/linkscan"
	"github.com/yatuk/tamga/internal/scanner/operator_state"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/telemetry"
	"github.com/yatuk/tamga/internal/upstream"
	"github.com/yatuk/tamga/internal/users"
	"github.com/yatuk/tamga/internal/vault"
	"github.com/yatuk/tamga/internal/webhooks"
)

func main() {
	// tamga mcp-wrap serves a local stdio MCP server over Streamable HTTP, so
	// that the gateway can stand in front of it (mcpwrap.go).
	if len(os.Args) > 1 && os.Args[1] == "mcp-wrap" {
		os.Exit(runMCPWrap(os.Args[2:], os.Stderr))
	}

	// Logger setup
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	started := time.Now()

	// Initialise OpenTelemetry tracing (noop unless OTLP endpoint is configured).
	tcfg := telemetry.ConfigFromEnv("tamga-proxy", buildinfo.String())
	telShutdown, telErr := telemetry.InitTracing(context.Background(), tcfg)
	if telErr != nil {
		log.Warn().Err(telErr).Msg("otlp exporter wiring failed; tracing disabled")
	} else if tcfg.Enabled && tcfg.Endpoint != "" {
		log.Info().
			Str("endpoint", tcfg.Endpoint).
			Float64("sample_rate", tcfg.SampleRate).
			Str("environment", tcfg.Environment).
			Msg("otel tracing enabled")
	}
	defer func() {
		if telShutdown == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = telShutdown(ctx)
	}()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// What a policy may ask of the proxy's environment and network: which
	// variables a provider pool or a notify value may name, and where a pool
	// may point. Set before the first load, so the file on disk, the watcher
	// and every API path are judged by the same rules.
	upstreamRules := &policy.UpstreamRules{
		KeyEnvs:        cfg.UpstreamKeyEnvs,
		NotifyEnvs:     cfg.NotifyURLEnvs,
		AllowHosts:     cfg.UpstreamAllowHosts,
		AllowNets:      cfg.UpstreamAllowNets,
		AllowHostPorts: cfg.UpstreamAllowHostPorts,
		RouteURLs:      proxy.ProviderURLOverrides(),
	}
	policy.SetUpstreamRules(upstreamRules)
	if len(cfg.UpstreamAllowNets)+len(cfg.UpstreamAllowHosts)+len(cfg.UpstreamAllowHostPorts) > 0 {
		nets := make([]string, len(cfg.UpstreamAllowNets))
		for i, p := range cfg.UpstreamAllowNets {
			nets[i] = p.String()
		}
		log.Warn().Strs("ranges", nets).Strs("hosts", cfg.UpstreamAllowHosts).Strs("host_ports", cfg.UpstreamAllowHostPorts).
			Msg("upstream: TAMGA_UPSTREAM_ALLOW lets provider pools in the policy send traffic to these hosts, internal ones and plain http included")
	}
	if len(cfg.UpstreamKeyEnvs) > 0 {
		log.Warn().Strs("names", cfg.UpstreamKeyEnvs).
			Msg("upstream: TAMGA_UPSTREAM_KEY_ENVS lets provider pools in the policy send these environment variables as a provider key")
	}
	if len(cfg.NotifyURLEnvs) > 0 {
		log.Warn().Strs("names", cfg.NotifyURLEnvs).
			Msg("alerts: TAMGA_NOTIFY_URL_ENVS lets a rule's notify value in the policy read its URL from these environment variables")
	}

	pol, err := policy.LoadFromFile(cfg.PolicyPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", cfg.PolicyPath).Msg("failed to load policy")
	}
	policyStore := policy.NewPolicyStore(pol)
	log.Info().Str("policy", pol.Name).Msg("policy loaded")
	policy.LogCoverageGaps(pol)

	// Declared here so the policy watcher callback can call Refresh();
	// assigned later when getCustomSpecs + patternStore are ready.
	var customScanner *scanner.CustomScanner
	var competitorScanner *scanner.CompetitorScanner
	var languageScanner *scanner.LanguageScanner
	var opScanner *operator_state.OperatorStateScanner

	// Set once the audit log and the policy history exist, further down. A
	// file change seen before that is reloaded but not recorded.
	var recordFileChange atomic.Pointer[func(old, new *policy.Policy)]

	var stopPolicyWatch func()
	if sw, err := policy.WatchPolicyChanges(cfg.PolicyPath, policyStore, func() {
		_ = scanner.ReloadDFA()
		if customScanner != nil {
			customScanner.Refresh()
		}
		if competitorScanner != nil {
			competitorScanner.Refresh()
		}
		if languageScanner != nil {
			languageScanner.Refresh()
		}
		if opScanner != nil {
			if p := policyStore.GetPolicy(); p != nil {
				if rules, err := operator_state.LoadRulesFromPolicy(p.OperatorState); err != nil {
					log.Warn().Err(err).Msg("operator_state: policy reload rejected; keeping previous rules")
				} else {
					opScanner.UpdateRules(rules)
				}
			}
		}
	}, func(old, new *policy.Policy) {
		// An edit of the file on disk has no actor; it is recorded as that.
		if record := recordFileChange.Load(); record != nil {
			(*record)(old, new)
		}
	}); err != nil {
		log.Warn().Err(err).Msg("policy file watcher disabled; restart required for policy changes")
	} else {
		stopPolicyWatch = sw
		log.Info().Str("path", cfg.PolicyPath).Msg("policy watcher started")
	}

	getPolicy := func() *policy.Policy { return policyStore.GetPolicy() }
	budgetStore := budget.New(getPolicy)
	promptCache := cache.New(2048)
	rateLimiter := ratelimit.NewLimiter(getPolicy)
	defer func() {
		if err := rateLimiter.Close(); err != nil {
			log.Warn().Err(err).Msg("rate limiter close")
		}
	}()

	// Optional Redis — when REDIS_URL is set and reachable, budget,
	// cache and rate-limiter all switch to distributed mode. Otherwise
	// everything keeps running on per-replica in-memory state.
	rdx := redisx.NewFromURL(cfg.RedisURL)
	if rdx.Enabled() {
		log.Info().Str("host", urlHost(cfg.RedisURL)).Msg("redis enabled — distributed mode")
		budgetStore.SetRedis(rdx)
		promptCache.SetRedis(rdx)
		rateLimiter.SetRedis(rdx)
	} else if cfg.RedisURL != "" {
		log.Warn().Str("host", urlHost(cfg.RedisURL)).Msg("redis URL set but unreachable; running single-node")

		// Tenant-scoped key namespacing (multi-tenant readiness).
		if cfg.DefaultOrgID != "" {
			rateLimiter.SetOrgID(cfg.DefaultOrgID)
		}

	}
	defer func() { _ = rdx.Close() }()

	// Vault: reversible PII tokenization store (AES-256-GCM at rest, Redis or
	// in-memory fallback). Enabled per-request by policy.Vault; here we just
	// build the cipher + store so the feature is available when toggled on.
	var vaultStore *vault.Store
	{
		keyB64 := cfg.VaultKey
		if keyB64 == "" {
			if gen, err := vault.GenerateKey(); err == nil {
				keyB64 = gen
				log.Warn().Msg("vault: TAMGA_VAULT_KEY unset — using an ephemeral key (mappings do not survive restart or cross-instance)")
			}
		}
		if key, err := vault.KeyFromBase64(keyB64); err != nil {
			log.Warn().Err(err).Msg("vault: invalid TAMGA_VAULT_KEY — vault disabled")
		} else if cipher, err := vault.NewCipher(key); err != nil {
			log.Warn().Err(err).Msg("vault: cipher init failed — vault disabled")
		} else {
			vaultStore = vault.NewStore(rdx, cipher, time.Duration(cfg.VaultTTLSeconds)*time.Second)
		}
	}

	// The key finding matches are hashed under when the policy has
	// data.hash_findings on: on every event, so in the request log, the
	// console's recent events, the live feed and the NATS stream alike.
	// Without one no digest is kept at all.
	if hasher, err := store.NewFindingHasher(cfg.FindingHashKey, cfg.VaultKey); err != nil {
		log.Error().Err(err).Msg("findings: no usable finding hash key; with data.hash_findings on, matches are not kept")
	} else if hasher == nil {
		log.Warn().Msg("findings: neither TAMGA_FINDING_HASH_KEY nor TAMGA_VAULT_KEY is set; with data.hash_findings on, matches are not kept and cannot be looked up")
	} else {
		store.SetFindingHasher(hasher)
		log.Info().Str("key_source", hasher.Source()).Msg("findings: matches are hashed with HMAC-SHA-256 when data.hash_findings is on")
	}

	var requestStore store.Store
	var pgStore *store.PostgresStore
	if cfg.DatabaseURL != "" {
		ps, err := store.NewPostgresStore(context.Background(), cfg.DatabaseURL, log.Logger)
		if err != nil {
			log.Warn().Err(err).Msg("postgres unavailable; using no-op request logging")
			requestStore = store.NewNoopStoreSilent()
		} else {
			pgStore = ps
			requestStore = ps
			log.Info().Msg("postgres request logging enabled")
			// The table may be older than this binary (the migrations are
			// run after the upgrade). Its newest columns are added here;
			// failing that, requests are stored without them.
			if err := ps.EnsureRequestLogColumns(context.Background()); err != nil {
				log.Warn().Err(err).Msg("request_logs could not be brought up to date; requests are stored without the newest columns until migration 017 is applied")
			}
			// Without an organisation nothing is stored and the saved hunts
			// API refuses every call. A single-tenant install has exactly one.
			if cfg.DefaultOrgID == "" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if id, err := ps.DefaultOrgID(ctx); err == nil && id != "" {
					cfg.DefaultOrgID = id
					log.Info().Str("org_id", id).Msg("TAMGA_ORG_ID not set; using the default organisation")
				} else {
					log.Warn().Err(err).Msg("TAMGA_ORG_ID not set and no default organisation found; requests will not be stored")
				}
				cancel()
			}
		}
	} else {
		requestStore = store.NewNoopStore(log.Logger)
	}
	defer func() {
		if err := requestStore.Close(); err != nil {
			log.Warn().Err(err).Msg("request store close")
		}
	}()

	// Model pricing — DB-backed with calculator cache (5-min TTL).
	// Falls back gracefully: when Postgres is off, pricing endpoints
	// return 503 and the hot-path pricing resolver uses the hardcoded
	// fallback map in proxy/pricing.go.
	var pricingStore *store.PricingStore
	var costCalculator *billing.Calculator
	var savedHuntStore store.SavedHuntStore
	if pgStore != nil {
		pricingStore = store.NewPricingStore(pgStore.Pool())
		costCalculator = billing.New(pricingStore, 5*time.Minute)
		savedHuntStore = store.NewSavedHuntStore(pgStore.Pool())
		log.Info().Msg("model pricing store + cost calculator + saved hunts wired")
	}

	// Data residency validation — warn when deployment region doesn't match policy.
	if cfg.Region != "" && pol.Data != nil && pol.Data.Residency != "" {
		if !strings.EqualFold(cfg.Region, pol.Data.Residency) {
			log.Warn().
				Str("config_region", cfg.Region).
				Str("policy_residency", pol.Data.Residency).
				Msg("data residency mismatch — policy requires " + pol.Data.Residency + " but running in " + cfg.Region)
		} else {
			log.Info().
				Str("region", cfg.Region).
				Msg("data residency validated")
		}
	}

	eventBus := events.NewBus()
	metrics := &events.Metrics{}
	recentBuf := events.NewRecentBuffer(1000)
	liveBroker := events.NewBroker(64)
	var incidentStore incidents.Store = incidents.NewMemoryStore()
	var incidentLifecycle incidents.LifecycleStore = incidents.NewMemoryStore()
	auditRing := incidents.NewAuditRing(512)
	if pgStore != nil {
		ap := incidents.NewAuditPersister(pgStore.Pool())
		pgIncident := incidents.NewPostgresStore(pgStore.Pool())
		if err := pgIncident.EnsureTable(context.Background()); err != nil {
			log.Warn().Err(err).Msg("incident_lifecycle table create failed; lifecycle persists in memory")
		} else {
			incidentStore = pgIncident
			incidentLifecycle = pgIncident
			log.Info().Msg("incident lifecycle backed by postgres")
		}
		if loaded, err := ap.Load(context.Background(), 512); err != nil {
			log.Warn().Err(err).Msg("audit log hydration failed; starting with empty ring")
		} else if len(loaded) > 0 {
			auditRing.Seed(loaded)
			log.Info().Int("entries", len(loaded)).Msg("audit log hydrated from postgres")
		}
		auditRing.SetPersister(func(e incidents.AuditEntry) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := ap.Append(ctx, e); err != nil {
				log.Warn().Err(err).Msg("audit log persist failed")
			}
		})
	}
	var apiKeyStore apikeys.Store = apikeys.NewMemoryStore()
	if pgStore != nil {
		if pgKeys, err := apikeys.NewPostgresStore(context.Background(), pgStore.Pool()); err != nil {
			log.Warn().Err(err).Msg("api keys: postgres store unavailable, keys are kept in memory and lost on restart")
		} else {
			apiKeyStore = pgKeys
			log.Info().Msg("api keys backed by postgres")
		}
	} else {
		log.Warn().Msg("api keys: no database configured, keys are kept in memory and lost on restart")
	}
	// One guard judges every address an alert is sent to: the probe, live
	// delivery and rule notify URLs. Internal addresses are refused unless
	// the operator listed them.
	alertGuard := webhooks.NewGuard(webhooks.GuardConfig{AllowNets: cfg.AlertAllowNets, AllowHosts: cfg.AlertAllowHosts})
	if len(cfg.AlertAllowNets)+len(cfg.AlertAllowHosts) > 0 {
		nets := make([]string, len(cfg.AlertAllowNets))
		for i, p := range cfg.AlertAllowNets {
			nets[i] = p.String()
		}
		log.Warn().Strs("ranges", nets).Strs("hosts", cfg.AlertAllowHosts).
			Msg("alerts: TAMGA_ALERT_ALLOW_PRIVATE lets alert destinations reach these internal addresses, over plain http too")
	}
	webhookStore := webhooks.NewMemoryStore()
	webhookStore.UseGuard(alertGuard)
	patternStore := patterns.NewMemoryStore()
	userStore := users.NewMemoryStore()
	// A pattern that arrives from another replica has to be compiled here
	// too. Without this the store would name it (and the cache key with it)
	// while the scanner went on without it.
	ssoStore := store.NewMemorySSOSettingsStore()
	persistSettings(cfg, pgStore, patternStore, userStore, webhookStore, ssoStore, func() {
		if customScanner != nil {
			customScanner.Refresh()
		}
	})
	clerkClient := users.NewClerkClient(cfg.ClerkSecretKey)
	if cfg.SSODefaultRole != "" {
		// Said at every start: with GitHub sign-in this lets in anyone who
		// has a GitHub account.
		log.Warn().Str("role", cfg.SSODefaultRole).
			Msg("TAMGA_SSO_DEFAULT_ROLE is set: an account that signs in without an assigned role gets this role instead of being refused")
	}
	var historyPool *pgxpool.Pool
	if pgStore != nil {
		historyPool = pgStore.Pool()
	}
	// Postgres with a database; files without one, and when the Postgres
	// store cannot be made.
	policyHistory, _ := policyhistory.Open(pgStore != nil, historyPool, os.Getenv("TAMGA_POLICY_HISTORY_DIR"))
	record := func(old, new *policy.Policy) {
		api.RecordPolicyFileChange(auditRing, policyHistory, cfg.PolicyPath, old, new)
	}
	recordFileChange.Store(&record)
	// NATS JetStream — fail-open. Proxy runs without NATS if unavailable.
	var natsPub *events.NATSPublisher
	if cfg.NATSURL != "" {
		np, err := events.NewNATSPublisher(context.Background(), events.NATSConfig{
			URL:    cfg.NATSURL,
			Stream: "TAMGA_EVENTS",
		}, log.Logger)
		if err != nil {
			log.Warn().Err(err).Str("host", urlHost(cfg.NATSURL)).Msg("NATS unavailable — running without JetStream")
		} else {
			natsPub = np
			defer np.Close()
		}
	}

	// In-memory handlers (fast path — always active):
	// data.hash_findings is applied to an event when it is published, before
	// any subscriber below has it.
	eventBus.SetPrepare(store.EventFindingHasher(log.Logger, getPolicy))
	eventBus.Subscribe(events.LogHandler(log.Logger))
	eventBus.Subscribe(events.MetricsHandler(metrics))

	// NATS bridge — publishes events for async DB/webhook consumers.
	// When NATS is unavailable, DBHandler stays on the in-memory bus.
	if natsPub != nil {
		eventBus.Subscribe(events.NewNATSHandler(natsPub).Handle)
	} else {
		eventBus.Subscribe(store.DBHandler(log.Logger, requestStore, cfg.DefaultOrgID, getPolicy))
	}

	// Connections to the internal gRPC services: each one's token, and TLS
	// when TAMGA_SERVICE_TLS_* is set. A TLS file that cannot be read stops
	// the start, as an invalid setting does.
	serviceTLS := grpcauth.ClientTLS{CAFile: cfg.ServiceTLSCA, CertFile: cfg.ServiceTLSCert, KeyFile: cfg.ServiceTLSKey}
	serviceDial := func(token string) []grpc.DialOption {
		opts, err := grpcauth.DialOptions(token, serviceTLS)
		if err != nil {
			log.Fatal().Err(err).Msg("internal service connection settings")
		}
		return opts
	}

	// Analyzer: deep NLP scan (stays in-memory for decision gating).
	analyzerClient, err := analyzer.NewGRPCClient(context.Background(), cfg.AnalyzerAddr, serviceDial(cfg.AnalyzerToken)...)
	if err != nil {
		log.Warn().Err(err).Str("addr", cfg.AnalyzerAddr).Msg("analyzer gRPC client unavailable — running without deep NLP")
	}
	// The one subscriber that is given a request's text (Event.Body, present
	// only under TAMGA_ANALYZER_SEND_CONTENT). Every other one gets the
	// event without it, so nothing that keeps events keeps the text. The
	// text is copied onto an event only when this handler will send it.
	events.SubscribeAnalyzer(eventBus, analyzerClient)

	// Inline classifier: on the decision path, unlike the analyzer above.
	var classifierGuard *classifier.Guard
	// The scan API asks the same classifier through a guard of its own, so
	// that failed scans cannot open the breaker the proxy path relies on.
	var scanClassifierGuard *classifier.Guard
	// Banned topics go to the same service, to its second model, through
	// guards of their own: a service without a topic model fails every
	// topic call, and that must not open the injection breaker.
	var topicGuard, scanTopicGuard *classifier.TopicGuard
	if classifierClient, err := classifier.NewClient(cfg.ClassifierAddr, serviceDial(cfg.ClassifierToken)...); err != nil {
		log.Warn().Err(err).Str("addr", cfg.ClassifierAddr).Msg("classifier client unavailable")
	} else if classifierClient != nil {
		classifierGuard = classifier.NewGuard(classifierClient)
		scanClassifierGuard = classifier.NewGuard(classifierClient)
		topicGuard = classifier.NewTopicGuard(classifierClient)
		scanTopicGuard = classifier.NewTopicGuard(classifierClient)
		api.SetClassifierStats(classifierGuard.Stats)
		api.SetTopicStats(topicGuard.Stats)
		go func() {
			hctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			h, err := classifierClient.HealthDetail(hctx)
			switch {
			case err != nil:
				log.Warn().Err(err).Str("addr", cfg.ClassifierAddr).Msg("classifier not reachable at startup")
				return
			case !h.Available:
				log.Warn().Str("reason", h.Reason).Msg("classifier has no model loaded")
			default:
				log.Info().Str("model", h.Model).Msg("classifier ready")
			}
			// Optional: only a policy with topics enabled needs it.
			if h.TopicsAvailable {
				log.Info().Str("model", h.TopicsModel).Msg("banned-topic model ready")
			} else {
				log.Info().Str("reason", h.TopicsReason).Msg("classifier has no banned-topic model loaded")
			}
			// Optional and off by default: a candidate scored in shadow,
			// recorded and never acted on.
			if h.ShadowAvailable {
				log.Info().Str("model", h.ShadowModel).Msg("classifier shadow model ready: its scores are recorded, not acted on")
			} else {
				log.Debug().Str("reason", h.ShadowReason).Msg("classifier has no shadow model loaded")
			}
		}()
	}
	eventBus.Subscribe(events.OperatorStateAnalyzerHandler(analyzerClient))
	eventBus.Subscribe(events.RecentBufferHandler(recentBuf))
	eventBus.Subscribe(liveBroker.Publish)
	// Decisions reach the saved alert destinations from here. The handler
	// only renders and queues; its own workers do the sending, so a slow
	// receiver cannot hold up the bus.
	alertDispatcher := webhooks.NewDispatcher(webhookStore, webhooks.DispatcherConfig{DefaultOrg: cfg.DefaultOrgID, Guard: alertGuard})
	api.SetAlertStats(alertDispatcher.Stats)
	eventBus.Subscribe(alertDispatcher.HandleEvent)
	eventBus.Start()

	// NATS consumers: DB persistence (async, durable, replayable).
	if natsPub != nil {
		go events.StartDurableConsumers(natsPub, map[string]struct {
			Subjects []string
			Handler  events.EventHandler
		}{
			"db-logger": {
				// Redacted requests are published as redact.applied; without it they were never stored.
				Subjects: []string{"scan.>", "block.>", "redact.>"},
				Handler: func(ev events.EventV2) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					storeEventFromNATS(ctx, requestStore, ev, cfg.DefaultOrgID, getPolicy, log.Logger)
				},
			},
		}, log.Logger)
	}

	upstreamReg := upstream.NewRegistry(upstream.Options{
		GetPolicy: getPolicy,
		Bus:       eventBus,
		Hooks: upstream.Hooks{
			BedrockSign: proxy.BedrockRequestSigner(),
		},
		// Pool endpoints come from the policy: they are dialled through
		// their own guarded transport, not the proxy's ordinary one.
		Egress: upstream.NewEgress(upstreamRules),
	})

	// Register scanners
	registry := scanner.NewRegistry()
	registry.Register(scanner.NewPIIScanner())
	registry.SetSpeed("pii", scanner.SpeedFast)
	registry.Register(scanner.NewSecretScanner())
	registry.SetSpeed("secret", scanner.SpeedFast)
	registry.Register(scanner.NewInjectionScanner())
	registry.SetSpeed("injection", scanner.SpeedSlow)
	registry.Register(scanner.NewJailbreakScanner())
	registry.SetSpeed("jailbreak", scanner.SpeedSlow)
	// Content moderation runs pure regex patterns inline — no external
	// service call needed. The analyzer has no scanner for it.
	registry.Register(scanner.NewContentModerationScanner())
	registry.SetSpeed("content_moderation", scanner.SpeedSlow)
	if err := scanner.InitDFA(); err != nil {
		log.Fatal().Err(err).Msg("DFA initialization failed")
	}
	getCustomSpecs := func() []scanner.CustomEntitySpec {
		p := policyStore.GetPolicy()
		out := make([]scanner.CustomEntitySpec, 0, 16)
		if p != nil {
			for _, ce := range p.CustomEntities {
				out = append(out, scanner.CustomEntitySpec{
					Name:        ce.Name,
					Pattern:     ce.Pattern,
					Description: ce.Description,
					Severity:    ce.Severity,
					Confidence:  ce.Confidence,
				})
			}
		}
		for _, up := range patternStore.List() {
			if !up.Enabled {
				continue
			}
			out = append(out, scanner.CustomEntitySpec{
				Name:        up.Name,
				Pattern:     up.Expression(),
				Description: "user-defined",
				Severity:    up.Severity,
				Confidence:  0.8,
			})
		}
		return out
	}
	// proxyBoundRegistry collects the scanners that depend on proxy-side state
	// (live policy, runtime patterns, request headers) and therefore keep
	// running in-process when scanning is delegated to the scanner-service.
	proxyBoundRegistry := scanner.NewRegistry()

	customScanner = scanner.NewCustomScanner(getCustomSpecs)
	registry.Register(customScanner)
	registry.SetSpeed("custom", scanner.SpeedFast)
	proxyBoundRegistry.Register(customScanner)
	proxyBoundRegistry.SetSpeed("custom", scanner.SpeedFast)

	getCompetitorSpecs := func() []scanner.CompetitorSpec {
		p := policyStore.GetPolicy()
		if p == nil {
			return nil
		}
		out := make([]scanner.CompetitorSpec, 0, len(p.Competitors))
		for _, c := range p.Competitors {
			out = append(out, scanner.CompetitorSpec{
				Name:     c.Name,
				Patterns: c.Patterns,
				Severity: c.Severity,
				Action:   c.Action,
				Enabled:  c.Enabled,
			})
		}
		return out
	}
	competitorScanner = scanner.NewCompetitorScanner(getCompetitorSpecs)
	registry.Register(competitorScanner)
	registry.SetSpeed("competitor", scanner.SpeedFast)
	proxyBoundRegistry.Register(competitorScanner)
	proxyBoundRegistry.SetSpeed("competitor", scanner.SpeedFast)

	// The language check reads the policy's language section at each
	// request. Its models are loaded only once a policy turns it on.
	languageScanner = scanner.NewLanguageScanner(func() scanner.LanguageSpec { return policyStore.GetPolicy().LanguageSpec() })
	languageScanner.Refresh()
	registry.Register(languageScanner)
	registry.SetSpeed(scanner.LanguageFindingType, scanner.SpeedFast)
	proxyBoundRegistry.Register(languageScanner)
	proxyBoundRegistry.SetSpeed(scanner.LanguageFindingType, scanner.SpeedFast)

	// Operator-state scanner (jugeni-contracts v1). Enabled when at least one
	// audit-log path is configured via TAMGA_OPERATOR_STATE_* env vars.
	if opCfg := operator_state.LoadConfig(); opCfg.Enabled {
		opProjection := operator_state.NewProjection()
		var opRedis *operator_state.RedisStore
		if opCfg.RedisEnabled && rdx.Enabled() {
			opRedis = operator_state.NewRedisStore(rdx)
			opProjection.SetRedisStore(opRedis)
		}

		opRules, err := operator_state.LoadRulesFromPolicy(pol.OperatorState)
		if err != nil {
			log.Fatal().Err(err).Msg("operator_state: invalid policy assertions")
		}
		opScanner = operator_state.NewOperatorStateScannerWithRules(opProjection, opRules)
		if opRedis != nil {
			opScanner.SetRedisStore(opRedis)
		}

		opWatcher, err := operator_state.NewWatcher(opCfg,
			func(ev operator_state.DecisionEvent) { opProjection.ApplyDecision(ev) },
			func(ev operator_state.NoteEvent) { opProjection.ApplyNote(ev) })
		if err != nil {
			log.Fatal().Err(err).Msg("operator_state: watcher init failed")
		}
		if err := opWatcher.Start(context.Background()); err != nil {
			log.Warn().Err(err).Msg("operator_state: initial audit-log replay incomplete (fail-open)")
		}
		defer opWatcher.Stop()
		if opRedis != nil {
			opRedis.SeedFromProjection(context.Background(), opProjection)
		}

		opScanner.SetDeepAnalysisPublisher(func(req *operator_state.AnalyzerRequest) {
			eventBus.Publish(events.Event{
				EventType: events.EventOperatorStateDeepScan,
				RequestID: req.RequestID,
				Metadata:  map[string]interface{}{"operator_state_request": req},
			})
		})

		registry.Register(opScanner)
		registry.SetSpeed("operator_state", scanner.SpeedFast)
		proxyBoundRegistry.Register(opScanner)
		proxyBoundRegistry.SetSpeed("operator_state", scanner.SpeedFast)
		decisions, notes := opProjection.Stats()
		log.Info().
			Int("decisions", decisions).
			Int("notes", notes).
			Bool("redis", opRedis != nil).
			Msg("operator_state scanner registered")
	}

	log.Info().Int("count", registry.Count()).Msg("scanners registered")

	// Output-only registry: scanners that should only run on response
	// bodies. The code leak scanner detects source code in LLM responses;
	// code in prompts is expected and should not be flagged.
	outputRegistry := scanner.NewRegistry()
	outputRegistry.Register(scanner.NewCodeLeakScanner())
	outputRegistry.SetSpeed("code_leak", scanner.SpeedFast)
	// The link scanner judges the links of an answer by their form and by
	// the policy's host lists, read from the policy in force at each scan.
	// It makes no request and resolves no name.
	outputRegistry.Register(scanner.NewLinkScanner(func() *linkscan.Lists { return getPolicy().LinkLists() }))
	outputRegistry.SetSpeed(scanner.LinkFindingType, scanner.SpeedFast)
	log.Info().Int("count", outputRegistry.Count()).Msg("output-only scanners registered")

	// Load BIN database: try embedded CSV first (zero-config, works in Docker),
	// fall back to file-based path for custom/override datasets.
	if err := scanner.InitBINLookupEmbed(); err != nil {
		log.Warn().Err(err).Msg("embedded BIN DB load failed, trying file-based fallback")
		binPath := os.Getenv("TAMGA_BIN_DB_PATH")
		if binPath == "" {
			binPath = "./data/bins.csv"
		}
		if _, err := os.Stat(binPath); err == nil {
			if err := scanner.InitBINLookup(binPath); err != nil {
				log.Warn().Err(err).Str("path", binPath).Msg("BIN DB file load failed — card detection continues without issuer validation")
			}
		} else {
			log.Info().Str("path", binPath).Msg("BIN DB file not found — card detection continues without issuer validation")
		}
	}

	// Demo-friendly startup banner (shown in terminal).
	providerForBanner := "anthropic"
	if !pol.ProviderAllowed("anthropic") && pol.ProviderAllowed("openai") {
		providerForBanner = "openai"
	}
	version := buildinfo.String()
	banner := fmt.Sprintf(
		"╔══════════════════════════════════════╗\n"+
			"║  TAMGA %s — AI Security Proxy        ║\n"+
			"║  Port: %d | Policy: %s            ║\n"+
			"║  Scanners: %d | Provider: %s      ║\n"+
			"╚══════════════════════════════════════╝",
		version, cfg.Port, pol.Name, registry.Count(), providerForBanner,
	)
	log.Info().Msg(banner)

	// Log active policy rule keys for quick demo narration.
	var ruleKeys []string
	for k := range pol.Rules {
		ruleKeys = append(ruleKeys, k)
	}
	sort.Strings(ruleKeys)
	log.Info().Strs("policy_rules", ruleKeys).Msg("policy rules loaded")

	retentionCtx, retentionCancel := context.WithCancel(context.Background())
	var retentionMgr *store.PartitionManager
	if pgStore != nil && retentionSchedulerEnabled() {
		retentionMgr = store.NewPartitionManager(pgStore.Pool(), store.RetentionPolicyFromEnv(), log.Logger)
		go func() {
			retentionMgr.Start(retentionCtx, 24*time.Hour)
		}()
		log.Info().Msg("retention scheduler started")
	}

	tlsEnabled := os.Getenv("TAMGA_TLS_CERT") != "" && os.Getenv("TAMGA_TLS_KEY") != ""
	mtlsEnabled := tlsEnabled && cfg.MTLSStrictVerify

	// Create scanner worker pool before API routes so metrics can reference it.
	var scannerPool *scanner.WorkerPool
	var scannerClient *scanner.GRPCScannerClient
	if cfg.ScannerWorkerPoolSize > 0 {
		queueSize := cfg.ScannerWorkerQueueSize
		if queueSize <= 0 {
			queueSize = cfg.ScannerWorkerPoolSize * 2
		}
		scannerPool = scanner.NewWorkerPool(cfg.ScannerWorkerPoolSize, queueSize)
		log.Info().
			Int("workers", cfg.ScannerWorkerPoolSize).
			Int("queue_size", queueSize).
			Msg("scanner worker pool started")
	}

	// Remote scanner service (gRPC client), enabled by TAMGA_SCANNER_SERVICE_ADDR
	// alone. The stateless scanners are delegated; proxy-bound scanners keep
	// running in-process (see proxyBoundRegistry). Fail-open: a gRPC error
	// falls back to the full local Registry for that request.
	if cfg.ScannerServiceAddr != "" {
		var scErr error
		scannerClient, scErr = scanner.NewGRPCScannerClient(context.Background(),
			scanner.GRPCScannerConfig{Addr: cfg.ScannerServiceAddr, DialOptions: serviceDial(cfg.ScannerServiceToken)})
		if scErr != nil {
			log.Warn().Err(scErr).Str("addr", cfg.ScannerServiceAddr).
				Msg("scanner gRPC client unavailable -- using local scanner registry")
		} else if scannerClient != nil {
			defer scannerClient.Close()
			log.Info().
				Str("addr", cfg.ScannerServiceAddr).
				Int("in_process_scanners", proxyBoundRegistry.Count()).
				Msg("scanning delegated to scanner-service; policy- and request-bound scanners stay in-process")
		}
	} else {
		log.Info().Msg("scanning in-process (TAMGA_SCANNER_SERVICE_ADDR not set)")
	}

	// Shared upstream transport — created once so TCP connections and
	// TLS sessions are reused across all requests. The per-request
	// approach of calling defaultStreamingTransport() was killing
	// performance and exhausting sockets under load.
	upstreamTransport := proxy.DefaultUpstreamTransport()

	proxyCfg := proxy.HandlerConfig{
		Registry:           registry,
		Keys:               apiKeyStore,
		Classifier:         classifierGuard,
		Topics:             topicGuard,
		OutputOnlyRegistry: outputRegistry,
		GetPolicy:          getPolicy,
		RateLimit:          rateLimiter,
		Config:             cfg,
		Bus:                eventBus,
		Alerts:             alertDispatcher,
		Budget:             budgetStore,
		Cache:              promptCache,
		// The cache key names the patterns the custom scanner is given, so a
		// pattern added or removed through the API is a miss, not a stale hit.
		// What the scanners have compiled, not what the policy and the
		// pattern store now say: between a change and the refresh that
		// follows it the two differ, and an answer scanned in that gap must
		// not be found under the key of the new rules.
		ScanState: func() string {
			state := ""
			if customScanner != nil {
				state = customScanner.Fingerprint()
			}
			if competitorScanner != nil {
				state += "/" + competitorScanner.Fingerprint()
			}
			return state
		},
		UpstreamRegistry:   upstreamReg,
		UpstreamTransport:  upstreamTransport,
		PricingResolver:    costCalculator,
		ScannerPool:        scannerPool,
		ScannerClient:      scannerClient,
		ProxyBoundRegistry: proxyBoundRegistry,
		VaultStore:         vaultStore,
	}

	// The scan API and the route LiteLLM calls as a guardrail: one scan path,
	// two request shapes.
	scanAPI, litellmBridge := proxy.NewScanHandlers(proxyCfg, proxy.ScanOptions{
		AdminKey:    cfg.AdminKey,
		Classifier:  scanClassifierGuard,
		Topics:      scanTopicGuard,
		CORSOrigins: cfg.CORSOrigins,
	})

	root := http.NewServeMux()
	root.Handle("/api/v1/", api.NewHandler(api.Config{
		AdminKey:               cfg.AdminKey,
		CORSOrigin:             cfg.CORSOrigin,
		CORSOrigins:            cfg.CORSOrigins,
		CORSAllowedMethods:     cfg.CORSAllowedMethods,
		CORSCredentials:        cfg.CORSCredentials,
		CORSMaxAge:             cfg.CORSMaxAge,
		PolicyPath:             cfg.PolicyPath,
		PolicyStore:            policyStore,
		DefaultOrgID:           cfg.DefaultOrgID,
		DatabaseURL:            cfg.DatabaseURL,
		Store:                  requestStore,
		Metrics:                metrics,
		Recent:                 recentBuf,
		Broker:                 liveBroker,
		Incidents:              incidentStore,
		IncidentLifecycle:      incidentLifecycle,
		Audit:                  auditRing,
		APIKeys:                apiKeyStore,
		Webhooks:               webhookStore,
		Patterns:               patternStore,
		Users:                  userStore,
		Clerk:                  clerkClient,
		PolicyHistory:          policyHistory,
		Budget:                 budgetStore,
		RateLimiter:            rateLimiter,
		ScannerCount:           registry.Count(),
		Started:                started,
		TLSEnabled:             tlsEnabled,
		MTLSEnabled:            mtlsEnabled,
		RedisEnabled:           rdx.Enabled(),
		RedixPing:              rdx.Ping,
		Bus:                    eventBus,
		Cache:                  promptCache,
		Version:                version,
		Upstream:               upstreamReg,
		TraceUIURL:             strings.TrimSpace(os.Getenv("TAMGA_TRACE_UI_URL")),
		Retention:              retentionMgr,
		SavedHunts:             savedHuntStore,
		PricingStore:           pricingStore,
		CostCalculator:         costCalculator,
		CustomScanner:          customScanner,
		CompetitorScanner:      competitorScanner,
		ScannerPool:            scannerPool,
		JWTSecret:              cfg.JWTSecret,
		GitHubClientID:         cfg.GitHubClientID,
		GitHubClientSecret:     cfg.GitHubClientSecret,
		GitHubOAuthCallbackURL: cfg.GitHubOAuthCallbackURL,
		SSODefaultRole:         cfg.SSODefaultRole,
		AnalyzerHTTPURL:        cfg.AnalyzerHTTPURL,
		AnalyzerToken:          cfg.AnalyzerToken,
		ClientAddr:             func(r *http.Request) string { return proxy.ClientIP(r, cfg.TrustedProxies) },
		SSOSettings:            ssoStore,
		Scan:                   scanAPI,
		LiteLLM:                litellmBridge,
	}))
	proxy.RegisterRoutes(root, proxyCfg)

	// The ICAP server for a company's web proxy, on a port of its own. Off
	// unless TAMGA_ICAP_ADDR is set.
	icapSrv := startICAP(cfg, scanAPI)
	// The MCP gateway, when TAMGA_MCP_CONFIG names its servers (docs/mcp.md).
	// A file that does not load stops the start: an agent would otherwise
	// find out by its tools going away.
	if mcpPath := strings.TrimSpace(os.Getenv("TAMGA_MCP_CONFIG")); mcpPath != "" {
		mcpCfg, err := proxy.LoadMCPGatewayConfig(mcpPath)
		if err != nil {
			log.Fatal().Err(err).Msg("mcp gateway config did not load")
		}
		proxy.RegisterMCPGateway(root, proxyCfg, mcpCfg)
		log.Info().Int("servers", len(mcpCfg.Servers)).Msg("mcp gateway serving /mcp/<name>")
	}

	// HTTP server
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second, // LLM + SSE streams can be slow
		IdleTimeout:       60 * time.Second,
	}

	// Optional TLS / mTLS configuration (Phase 2F — Turkey regulated workloads
	// typically require TLS 1.2+ and client-cert auth for management APIs).
	tlsCert := os.Getenv("TAMGA_TLS_CERT")
	tlsKey := os.Getenv("TAMGA_TLS_KEY")
	// mTLS client CA now sourced from cfg.MTLSClientCAFile (TAMGA_MTLS_CLIENT_CA_FILE env).
	if tlsCert != "" && tlsKey != "" {
		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		}
		if cfg.MTLSStrictVerify {
			if cfg.MTLSClientCAFile == "" {
				log.Fatal().Msg("mTLS strict verify enabled but TAMGA_MTLS_CLIENT_CA_FILE is not set")
			}
			pool := x509.NewCertPool()
			caBytes, err := os.ReadFile(cfg.MTLSClientCAFile)
			if err != nil {
				log.Fatal().Err(err).Str("ca_file", cfg.MTLSClientCAFile).Msg("mTLS CA read failed")
			}
			if !pool.AppendCertsFromPEM(caBytes) {
				log.Fatal().Str("ca_file", cfg.MTLSClientCAFile).Msg("mTLS CA has no valid certs")
			}
			tlsCfg.ClientCAs = pool
			tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
			log.Info().Str("ca_file", cfg.MTLSClientCAFile).Msg("mTLS enabled — client certs required")
		}
		srv.TLSConfig = tlsCfg
	}

	// Graceful shutdown
	go func() {
		log.Info().Int("port", cfg.Port).Msg("tamga proxy starting")
		var err error
		if srv.TLSConfig != nil {
			log.Info().Msg("starting with TLS")
			err = srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	retentionCancel()
	log.Info().Msg("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if icapSrv != nil {
		if err := icapSrv.Shutdown(ctx); err != nil {
			log.Warn().Err(err).Msg("icap server shutdown timed out")
		}
	}
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("forced shutdown")
	}
	eventBus.Stop()
	// After the bus, so the last decisions are queued before the drain.
	alertDispatcher.Stop(3 * time.Second)
	if analyzerClient != nil {
		if err := analyzerClient.Close(); err != nil {
			log.Warn().Err(err).Msg("analyzer gRPC client close")
		}
	}
	log.Info().
		Int64("metric_requests", metrics.TotalRequests.Load()).
		Int64("metric_blocked", metrics.Blocked.Load()).
		Int64("metric_redacted", metrics.Redacted.Load()).
		Int64("metric_warned", metrics.Warned.Load()).
		Msg("event bus stopped")
	if stopPolicyWatch != nil {
		stopPolicyWatch()
		log.Info().Msg("policy watcher stopped")
	}
	// Shut down scanner worker pool with generous timeout.
	if scannerPool != nil {
		log.Info().Msg("shutting down scanner worker pool...")
		if err := scannerPool.Shutdown(30 * time.Second); err != nil {
			log.Warn().Err(err).Msg("scanner pool shutdown timed out")
		} else {
			log.Info().Msg("scanner worker pool stopped")
		}
	}
	// requestStore.Close already deferred
	log.Info().Msg("tamga proxy stopped")
}

// storeEventFromNATS persists a NATS EventV2 to PostgreSQL. This runs in the
// NATS consumer goroutine, decoupled from the HTTP request path.
//
// The row is the one the in-process bus would have written: both build it
// with store.RequestLogFromEvent. Events that are not a request's own share
// its subject (output hints, provider state changes) and are not rows.
func storeEventFromNATS(ctx context.Context, s store.Store, ev events.EventV2, defaultOrgID string, getPolicy func() *policy.Policy, log zerolog.Logger) {
	before := store.UnkeyedMatches()
	rl, ok := store.RequestLogFromV2(ev, defaultOrgID, getPolicy)
	if !ok {
		return
	}
	// Only for an envelope the bus did not hash, and once per process.
	if store.UnkeyedMatches() != before {
		store.LogUnkeyedMatches(log)
	}
	if err := s.SaveRequestLog(ctx, rl); err != nil {
		log.Warn().Err(err).Str("request_id", ev.RequestID).Msg("nats consumer: db write failed")
	}
}

// urlHost is what of a connection URL may be written to a log: where it
// points. The URL itself carries the password.
func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "unparseable"
	}
	return u.Host
}

func retentionSchedulerEnabled() bool {
	e := strings.TrimSpace(os.Getenv("TAMGA_RETENTION_ENABLED"))
	return e == "1" || strings.EqualFold(e, "true")
}

// settingsReloadInterval is how long a change to patterns, team roles,
// SSO settings or webhooks made on one replica takes to reach the others.
// A revoked session is such a change (it is stored with the team roles): a
// token revoked on one replica is honoured by another for up to this long.
const settingsReloadInterval = 30 * time.Second

// persistSettings puts the database behind the pattern, team-role, SSO and
// webhook stores, so they survive a restart and agree across replicas.
// Without a database they stay in memory, as before.
//
// afterReload runs after each periodic reload, for whoever keeps something
// compiled from these stores.
func persistSettings(cfg *config.Config, pg *store.PostgresStore, pats *patterns.MemoryStore, team *users.MemoryStore, hooks *webhooks.MemoryStore, sso *store.MemorySSOSettingsStore, afterReload func()) {
	if pg == nil {
		log.Warn().Msg("settings: no database configured, custom patterns, team roles, SSO settings and webhooks are kept in memory and lost on restart")
		return
	}
	pool := pg.Pool()
	if err := docstore.Init(context.Background(), pool); err != nil {
		log.Warn().Err(err).Msg("settings: database unavailable, custom patterns, team roles, SSO settings and webhooks are kept in memory and lost on restart")
		return
	}
	type reloadable interface{ Reload() error }
	var live []reloadable

	if err := pats.Persist(docstore.NewTable(pool, "pattern", nil)); err != nil {
		log.Warn().Err(err).Msg("settings: custom patterns could not be loaded")
	}
	live = append(live, pats)
	if err := team.Persist(docstore.NewTable(pool, "team_role", nil)); err != nil {
		log.Warn().Err(err).Msg("settings: team roles could not be loaded")
	}
	live = append(live, team)
	// The SSO settings hold no secret (see store.SSOSettings), so like the
	// team roles they are stored as they are and need no vault key.
	if err := sso.Persist(docstore.NewTable(pool, "sso_settings", nil)); err != nil {
		log.Warn().Err(err).Msg("settings: SSO settings could not be loaded")
	}
	live = append(live, sso)

	// A webhook holds a URL and often a token that are credentials for
	// someone else's system. They go to the database encrypted or not at
	// all, and the ephemeral vault key would not survive the restart.
	if cfg.VaultKey == "" {
		log.Warn().Msg("settings: TAMGA_VAULT_KEY is not set, webhooks are kept in memory and lost on restart")
	} else if key, err := vault.KeyFromBase64(cfg.VaultKey); err != nil {
		log.Warn().Err(err).Msg("settings: invalid TAMGA_VAULT_KEY, webhooks are kept in memory and lost on restart")
	} else if seal, err := vault.NewCipher(key); err != nil {
		log.Warn().Err(err).Msg("settings: cipher init failed, webhooks are kept in memory and lost on restart")
	} else {
		if err := hooks.Persist(docstore.NewTable(pool, "webhook", seal)); err != nil {
			log.Warn().Err(err).Msg("settings: webhooks could not be loaded")
		}
		live = append(live, hooks)
	}
	log.Info().Int("patterns", len(pats.List())).Int("team_roles", len(team.List())).Int("webhooks", len(hooks.List())).
		Msg("settings backed by postgres")

	go func() {
		t := time.NewTicker(settingsReloadInterval)
		defer t.Stop()
		for range t.C {
			for _, s := range live {
				if err := s.Reload(); err != nil {
					log.Warn().Err(err).Msg("settings: reload failed, keeping the current copy")
				}
			}
			if afterReload != nil {
				afterReload()
			}
		}
	}()
}
