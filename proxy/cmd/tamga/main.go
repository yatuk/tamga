package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/analyzer"
	"github.com/yatuk/tamga/internal/api"
	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/billing"
	"github.com/yatuk/tamga/internal/budget"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/patterns"
	"github.com/yatuk/tamga/internal/policy"
	policyhistory "github.com/yatuk/tamga/internal/policy/history"
	"github.com/yatuk/tamga/internal/proxy"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/redisx"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/secrets"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/telemetry"
	"github.com/yatuk/tamga/internal/tier"
	"github.com/yatuk/tamga/internal/upstream"
	"github.com/yatuk/tamga/internal/users"
	"github.com/yatuk/tamga/internal/webhooks"
)

func main() {
	// Logger setup
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	started := time.Now()

	// Initialise OpenTelemetry tracing (noop unless OTLP endpoint is configured).
	tcfg := telemetry.ConfigFromEnv("tamga-proxy", "v0.1.1")
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

	// Secrets provider — Vault integration (optional, disabled by default).
	// When TAMGA_VAULT_ADDR is set, creates a VaultProvider with env-var fallback.
	// When empty (default), uses EnvProvider (os.Getenv) — backward compatible.
	secretProvider, secretsErr := secrets.NewFromConfig()
	if secretsErr != nil {
		log.Fatal().Err(secretsErr).Msg("failed to initialize secrets provider")
	}
	if secretProvider.Enabled() {
		log.Info().Str("vault_addr", cfg.VaultAddr).Msg("vault secrets provider enabled — secrets will be resolved from Vault")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := secretProvider.HealthCheck(ctx); err != nil {
			log.Warn().Err(err).Msg("vault unreachable at startup — falling back to env vars")
		}
		cancel()
	}
	defer func() {
		if err := secretProvider.Close(); err != nil {
			log.Warn().Err(err).Msg("secrets provider close")
		}
	}()

	pol, err := policy.LoadFromFile(cfg.PolicyPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", cfg.PolicyPath).Msg("failed to load policy")
	}
	policyStore := policy.NewPolicyStore(pol)
	log.Info().Str("policy", pol.Name).Msg("policy loaded")

	// Declared here so the policy watcher callback can call Refresh();
	// assigned later when getCustomSpecs + patternStore are ready.
	var customScanner *scanner.CustomScanner
	var competitorScanner *scanner.CompetitorScanner
	var tierEnforcer *tier.Enforcer

	var stopPolicyWatch func()
	if sw, err := policy.WatchPolicy(cfg.PolicyPath, policyStore, func() {
		_ = scanner.ReloadDFA()
		if customScanner != nil {
			customScanner.Refresh()
		}
		if competitorScanner != nil {
			competitorScanner.Refresh()
		}
		if tierEnforcer != nil {
			tierEnforcer.Refresh()
		}
	}); err != nil {
		log.Warn().Err(err).Msg("policy file watcher disabled; restart required for policy changes")
	} else {
		stopPolicyWatch = sw
		log.Info().Str("path", cfg.PolicyPath).Msg("policy watcher started")
	}

	getPolicy := func() *policy.Policy { return policyStore.GetPolicy() }
	tierEnforcer = tier.New(getPolicy)
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
		log.Info().Str("url", cfg.RedisURL).Msg("redis enabled — distributed mode")
		budgetStore.SetRedis(rdx)
		promptCache.SetRedis(rdx)
		rateLimiter.SetRedis(rdx)
	} else if cfg.RedisURL != "" {
		log.Warn().Str("url", cfg.RedisURL).Msg("redis URL set but unreachable; running single-node")

		// Tenant-scoped key namespacing (multi-tenant readiness).
		if cfg.DefaultOrgID != "" {
			rateLimiter.SetOrgID(cfg.DefaultOrgID)
		}

	}
	defer func() { _ = rdx.Close() }()

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

	// Air-gapped deployment validation — warn when policy requires air-gapped
	// but the deployment may not be (on-prem flag is set via TAMGA_AIR_GAPPED env).
	if tierEnforcer != nil && tierEnforcer.AirGappedAllowed() {
		log.Info().Msg("tier permits air-gapped / on-prem deployment")
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

	// Pricing tier validation — warn when deployment tier doesn't match policy active tier.
	if pol.Pricing != nil {
		if cfg.Tier != "" && pol.Pricing.ActiveTier != "" && !strings.EqualFold(cfg.Tier, pol.Pricing.ActiveTier) {
			log.Warn().
				Str("config_tier", cfg.Tier).
				Str("policy_active_tier", pol.Pricing.ActiveTier).
				Msg("pricing tier mismatch — policy expects " + pol.Pricing.ActiveTier + " but running as " + cfg.Tier)
		}
		if td := pol.Pricing.ActiveTierDef(); td != nil {
			log.Info().
				Str("tier", td.Name).
				Int("max_requests_mo", td.MaxRequestsMo).
				Bool("sso", td.SSOEnabled).
				Int("retention_days", td.RetentionDays).
				Msg("pricing tier enforced")
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
	apiKeyStore := apikeys.NewMemoryStore()
	webhookStore := webhooks.NewMemoryStore()
	patternStore := patterns.NewMemoryStore()
	userStore := users.NewMemoryStore()
	ssoStore := store.NewMemorySSOSettingsStore()
	clerkClient := users.NewClerkClient(cfg.ClerkSecretKey)
	var policyHistory policyhistory.Store
	if pgStore != nil {
		pgHist, err := policyhistory.NewPostgresStore(pgStore.Pool())
		if err != nil {
			log.Warn().Err(err).Msg("policy history postgres init failed, falling back to file")
		} else {
			policyHistory = pgHist
			log.Info().Msg("policy history backed by postgres")
		}
		log.Info().Msg("policy history backed by postgres")
	} else {
		historyDir := os.Getenv("TAMGA_POLICY_HISTORY_DIR")
		if historyDir == "" {
			historyDir = "./data/policy-history"
		}
		fs, err := policyhistory.NewFileStore(historyDir)
		if err != nil {
			log.Warn().Err(err).Str("dir", historyDir).Msg("policy history disabled")
		} else {
			policyHistory = fs
		}
	}
	// NATS JetStream — fail-open. Proxy runs without NATS if unavailable.
	var natsPub *events.NATSPublisher
	if cfg.NATSURL != "" {
		np, err := events.NewNATSPublisher(context.Background(), events.NATSConfig{
			URL:    cfg.NATSURL,
			Stream: "TAMGA_EVENTS",
		}, log.Logger)
		if err != nil {
			log.Warn().Err(err).Str("url", cfg.NATSURL).Msg("NATS unavailable — running without JetStream")
		} else {
			natsPub = np
			defer np.Close()
		}
	}

	// In-memory handlers (fast path — always active):
	eventBus.Subscribe(events.LogHandler(log.Logger))
	eventBus.Subscribe(events.MetricsHandler(metrics))

	// NATS bridge — publishes events for async DB/webhook consumers.
	// When NATS is unavailable, DBHandler stays on the in-memory bus.
	if natsPub != nil {
		eventBus.Subscribe(events.NewNATSHandler(natsPub).Handle)
	} else {
		eventBus.Subscribe(store.DBHandler(log.Logger, requestStore, cfg.DefaultOrgID, getPolicy))
	}

	// Analyzer: deep NLP scan (stays in-memory for decision gating).
	analyzerClient, err := analyzer.NewGRPCClient(context.Background(), cfg.AnalyzerAddr)
	if err != nil {
		log.Warn().Err(err).Str("addr", cfg.AnalyzerAddr).Msg("analyzer gRPC client unavailable — running without deep NLP")
	}
	eventBus.Subscribe(events.AnalyzerHandler(analyzerClient))
	eventBus.Subscribe(events.RecentBufferHandler(recentBuf))
	eventBus.Subscribe(liveBroker.Publish)
	eventBus.Start()

	// NATS consumers: DB persistence (async, durable, replayable).
	if natsPub != nil {
		go events.StartDurableConsumers(natsPub, map[string]struct {
			Subjects []string
			Handler  events.EventHandler
		}{
			"db-logger": {
				Subjects: []string{"scan.>", "block.>"},
				Handler: func(ev events.EventV2) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					storeEventFromNATS(ctx, requestStore, ev, log.Logger)
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
	// service call needed. Offloads Python toxicity.py heuristic fallback.
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
			if up.Kind != patterns.KindRegex {
				// Literal kind is compiled into regex by anchoring escape.
				continue
			}
			out = append(out, scanner.CustomEntitySpec{
				Name:        up.Name,
				Pattern:     up.Pattern,
				Description: "user-defined",
				Severity:    up.Severity,
				Confidence:  0.8,
			})
		}
		return out
	}
	customScanner = scanner.NewCustomScanner(getCustomSpecs)
	registry.Register(customScanner)
	registry.SetSpeed("custom", scanner.SpeedFast)

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
	log.Info().Int("count", registry.Count()).Msg("scanners registered")

	// Output-only registry: scanners that should only run on response
	// bodies. The code leak scanner detects source code in LLM responses;
	// code in prompts is expected and should not be flagged.
	outputRegistry := scanner.NewRegistry()
	outputRegistry.Register(scanner.NewCodeLeakScanner())
	outputRegistry.SetSpeed("code_leak", scanner.SpeedFast)
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
	version := "v0.1.0"
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

		// Remote scanner service (gRPC client). Fail-open: falls back to local Registry.
		var scErr error
		scannerClient, scErr = scanner.NewGRPCScannerClient(context.Background(),
			scanner.GRPCScannerConfig{Addr: cfg.ScannerServiceAddr})
		if scErr != nil {
			log.Warn().Err(scErr).Str("addr", cfg.ScannerServiceAddr).
				Msg("scanner gRPC client unavailable -- using local scanner registry")
		} else if scannerClient != nil {
			defer scannerClient.Close()
		}

	}

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
		TierEnforcer:           tierEnforcer,
		ScannerPool:            scannerPool,
		JWTSecret:              cfg.JWTSecret,
		GitHubClientID:         cfg.GitHubClientID,
		GitHubClientSecret:     cfg.GitHubClientSecret,
		GitHubOAuthCallbackURL: cfg.GitHubOAuthCallbackURL,
		ClerkSecretKey:         cfg.ClerkSecretKey,
		ClerkCallbackURL:       cfg.ClerkCallbackURL,
		AnalyzerHTTPURL:        cfg.AnalyzerHTTPURL,
		SSOSettings:            ssoStore,
	}))
	// Shared upstream transport — created once so TCP connections and
	// TLS sessions are reused across all requests. The per-request
	// approach of calling defaultStreamingTransport() was killing
	// performance and exhausting sockets under load.
	upstreamTransport := proxy.DefaultUpstreamTransport()

	proxy.RegisterRoutes(root, proxy.HandlerConfig{
		Registry:           registry,
		OutputOnlyRegistry: outputRegistry,
		GetPolicy:          getPolicy,
		RateLimit:          rateLimiter,
		Config:             cfg,
		Bus:                eventBus,
		Budget:             budgetStore,
		Cache:              promptCache,
		UpstreamRegistry:   upstreamReg,
		UpstreamTransport:  upstreamTransport,
		PricingResolver:    costCalculator,
		TierEnforcer:       tierEnforcer, // nil-safe — falls back to hardcoded map
		ScannerPool:        scannerPool,
		ScannerClient:      scannerClient,
	})

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
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("forced shutdown")
	}
	eventBus.Stop()
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
func storeEventFromNATS(ctx context.Context, s store.Store, ev events.EventV2, log zerolog.Logger) {
	rl := store.RequestLog{
		RequestID:    ev.RequestID,
		OrgID:        ev.OrgID,
		Provider:     stringField(ev.Payload, "provider"),
		Model:        stringField(ev.Payload, "model"),
		InputTokens:  intField(ev.Payload, "input_tokens"),
		OutputTokens: intField(ev.Payload, "output_tokens"),
		ActionTaken:  stringField(ev.Payload, "action"),
		UserID:       stringField(ev.Payload, "user_id"),
	}
	if raw, ok := ev.Payload["findings"]; ok {
		var findings []scanner.Finding
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &findings)
			rl.Findings, _ = json.Marshal(findings)
			rl.FindingsCount = len(findings)
		}
	}
	if err := s.SaveRequestLog(ctx, rl); err != nil {
		log.Warn().Err(err).Str("request_id", ev.RequestID).Msg("nats consumer: db write failed")
	}
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		s, _ := v.(string)
		return s
	}
	return ""
}

func intField(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

func retentionSchedulerEnabled() bool {
	e := strings.TrimSpace(os.Getenv("TAMGA_RETENTION_ENABLED"))
	return e == "1" || strings.EqualFold(e, "true")
}
