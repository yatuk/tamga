package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/telemetry"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RetentionPolicy configures how long different datasets are kept (days).
type RetentionPolicy struct {
	RequestLogsDays    int
	AlertsDays         int
	DailyStatsDays     int
	AuditLogsDays      int
	IncidentClosedDays int // how long closed incidents stay before purge
}

// DefaultRetentionPolicy returns conservative defaults.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		RequestLogsDays:    30,
		AlertsDays:         90,
		DailyStatsDays:     365,
		AuditLogsDays:      365,
		IncidentClosedDays: 0,
	}
}

// RetentionPolicyFromEnv loads TAMGA_RETENTION_* overrides.
func RetentionPolicyFromEnv() RetentionPolicy {
	p := DefaultRetentionPolicy()
	if v := envIntPositive("TAMGA_RETENTION_REQUEST_LOGS_DAYS"); v > 0 {
		p.RequestLogsDays = v
	}
	if v := envIntPositive("TAMGA_RETENTION_ALERTS_DAYS"); v > 0 {
		p.AlertsDays = v
	}
	if v := envIntPositive("TAMGA_RETENTION_DAILY_STATS_DAYS"); v > 0 {
		p.DailyStatsDays = v
	}
	if v := envIntPositive("TAMGA_RETENTION_AUDIT_DAYS"); v > 0 {
		p.AuditLogsDays = v
	}
	if v := envIntPositive("TAMGA_RETENTION_INCIDENT_CLOSED_DAYS"); v > 0 {
		p.IncidentClosedDays = v
	}
	return p
}

func envIntPositive(key string) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// PartitionManager maintains monthly request_logs partitions and purges row-based tables.
type PartitionManager struct {
	db     *pgxpool.Pool
	policy RetentionPolicy
	log    zerolog.Logger
	tracer trace.Tracer

	lastRunUnix atomic.Int64
}

// NewPartitionManager builds a manager.
func NewPartitionManager(db *pgxpool.Pool, policy RetentionPolicy, log zerolog.Logger) *PartitionManager {
	return &PartitionManager{
		db:     db,
		policy: policy,
		log:    log.With().Str("component", "retention").Logger(),
		tracer: telemetry.Tracer(),
	}
}

// LastRun returns the last successful maintenance time (UTC), or zero if none.
func (pm *PartitionManager) LastRun() time.Time {
	u := pm.lastRunUnix.Load()
	if u == 0 {
		return time.Time{}
	}
	return time.Unix(u, 0).UTC()
}

// RunMaintenanceCycle creates upcoming partitions, drops expired ones, purges old rows, analyzes.
func (pm *PartitionManager) RunMaintenanceCycle(ctx context.Context) error {
	if pm == nil || pm.db == nil {
		return fmt.Errorf("retention: nil manager")
	}
	ctx, span := pm.tracer.Start(ctx, "retention.cycle")
	defer span.End()

	if err := pm.createUpcomingPartitions(ctx, 6); err != nil {
		span.RecordError(err)
		pm.logRun(ctx, "create_partitions", err.Error(), nil)
		return fmt.Errorf("create partitions: %w", err)
	}
	pm.logRun(ctx, "create_partitions", "ok", nil)

	if err := pm.dropExpiredPartitions(ctx); err != nil {
		span.RecordError(err)
		pm.logRun(ctx, "drop_partitions", err.Error(), nil)
		return fmt.Errorf("drop partitions: %w", err)
	}
	pm.logRun(ctx, "drop_partitions", "ok", nil)

	if err := pm.purgeOldRows(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		pm.logRun(ctx, "purge_rows", err.Error(), nil)
		return fmt.Errorf("purge rows: %w", err)
	}
	pm.logRun(ctx, "purge_rows", "ok", nil)

	if err := pm.analyzeRequestLogs(ctx); err != nil {
		pm.log.Warn().Err(err).Str("component", "store").Msg("analyze request_logs failed")
		pm.logRun(ctx, "analyze", err.Error(), nil)
	} else {
		pm.logRun(ctx, "analyze", "ok", nil)
	}

	pm.lastRunUnix.Store(time.Now().UTC().Unix())
	return nil
}

func (pm *PartitionManager) logRun(ctx context.Context, phase, msg string, detail map[string]interface{}) {
	pm.log.Info().Str("component", "store").Str("phase", phase).Str("msg", msg).Msg("retention step")
	raw := []byte("{}")
	if len(detail) > 0 {
		b, err := json.Marshal(detail)
		if err == nil {
			raw = b
		}
	}
	_, err := pm.db.Exec(ctx, `
INSERT INTO retention_run_log (phase, message, detail) VALUES ($1, $2, $3::jsonb)`,
		phase, nullStr(msg), raw)
	if err != nil {
		pm.log.Warn().Err(err).Str("component", "store").Str("phase", phase).Msg("retention_run_log insert skipped")
	}
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// Start runs RunMaintenanceCycle immediately then every interval (default 24h).
func (pm *PartitionManager) Start(ctx context.Context, interval time.Duration) {
	if pm == nil || pm.db == nil {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if err := pm.RunMaintenanceCycle(ctx); err != nil {
		pm.log.Error().Err(err).Str("component", "store").Msg("initial retention cycle failed")
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := pm.RunMaintenanceCycle(ctx); err != nil {
				pm.log.Error().Err(err).Str("component", "store").Msg("retention cycle failed")
			}
		}
	}
}

func (pm *PartitionManager) createUpcomingPartitions(ctx context.Context, monthsAhead int) error {
	now := time.Now().UTC()
	for i := 0; i < monthsAhead; i++ {
		m := now.AddDate(0, i, 0)
		y, mo := m.Year(), m.Month()
		partName := fmt.Sprintf("request_logs_%04d_%02d", y, mo)
		start := time.Date(y, mo, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		q := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s PARTITION OF request_logs
FOR VALUES FROM ('%s') TO ('%s')`,
			partName,
			start.Format("2006-01-02"),
			end.Format("2006-01-02"),
		)
		if _, err := pm.db.Exec(ctx, q); err != nil {
			return fmt.Errorf("create partition %s: %w", partName, err)
		}
		pm.log.Info().Str("component", "store").Str("partition", partName).Msg("partition ensured")
	}
	return nil
}

var rePartTO = regexp.MustCompile(`TO\s*\(\s*'([^']+)'\s*\)`)

func (pm *PartitionManager) dropExpiredPartitions(ctx context.Context) error {
	if pm.policy.RequestLogsDays <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -pm.policy.RequestLogsDays)

	rows, err := pm.db.Query(ctx, `
SELECT c.relname::text, pg_get_expr(c.relpartbound, c.oid)::text
FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'request_logs'
  AND c.relname <> 'request_logs_default'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type part struct {
		name, bound string
	}
	var list []part
	for rows.Next() {
		var name, bound string
		if err := rows.Scan(&name, &bound); err != nil {
			return err
		}
		list = append(list, part{name: name, bound: bound})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range list {
		toTime, ok := parsePartitionUpperBoundTO(p.bound)
		if !ok {
			pm.log.Warn().Str("component", "store").Str("partition", p.name).Str("bound", p.bound).Msg("skip partition: could not parse TO bound")
			continue
		}
		// All rows in partition have created_at < toTime. Safe to drop if upper bound <= cutoff.
		if toTime.After(cutoff) {
			continue
		}
		if err := pm.detachDrop(ctx, p.name); err != nil {
			return fmt.Errorf("drop %s: %w", p.name, err)
		}
		pm.log.Info().Str("component", "store").Str("partition", p.name).Time("upper_bound", toTime).Msg("partition dropped (retention)")
	}
	return nil
}

func parsePartitionUpperBoundTO(bound string) (time.Time, bool) {
	m := rePartTO.FindStringSubmatch(bound)
	if len(m) < 2 {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", m[1], time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (pm *PartitionManager) detachDrop(ctx context.Context, partName string) error {
	// Simple DETACH — brief exclusive lock on parent.
	q1 := fmt.Sprintf(`ALTER TABLE request_logs DETACH PARTITION %s`, partName)
	if _, err := pm.db.Exec(ctx, q1); err != nil {
		return err
	}
	q2 := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, partName)
	_, err := pm.db.Exec(ctx, q2)
	return err
}

func (pm *PartitionManager) purgeOldRows(ctx context.Context) error {
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if pm.policy.AlertsDays > 0 {
		cut := time.Now().UTC().AddDate(0, 0, -pm.policy.AlertsDays)
		tag, err := pm.db.Exec(ctx2, `DELETE FROM alerts WHERE created_at < $1`, cut)
		if err != nil {
			return fmt.Errorf("alerts: %w", err)
		}
		pm.log.Info().Str("component", "store").Int64("rows", tag.RowsAffected()).Msg("purged alerts")
	}

	if pm.policy.DailyStatsDays > 0 {
		cut := time.Now().UTC().AddDate(0, 0, -pm.policy.DailyStatsDays)
		tag, err := pm.db.Exec(ctx2, `DELETE FROM daily_stats WHERE stat_date < $1::date`, cut)
		if err != nil {
			return fmt.Errorf("daily_stats: %w", err)
		}
		pm.log.Info().Str("component", "store").Int64("rows", tag.RowsAffected()).Msg("purged daily_stats")
	}

	if pm.policy.AuditLogsDays > 0 {
		cut := time.Now().UTC().AddDate(0, 0, -pm.policy.AuditLogsDays)
		tag, err := pm.db.Exec(ctx2, `DELETE FROM audit_log WHERE ts < $1`, cut)
		if err != nil {
			// Table may not exist on very old DBs
			pm.log.Warn().Err(err).Str("component", "store").Msg("audit_log purge skipped or failed")
		} else {
			pm.log.Info().Str("component", "store").Int64("rows", tag.RowsAffected()).Msg("purged audit_log")
		}
	}

	if pm.policy.IncidentClosedDays > 0 {
		cut := time.Now().UTC().AddDate(0, 0, -pm.policy.IncidentClosedDays)
		tag, err := pm.db.Exec(ctx2, `DELETE FROM incident_lifecycle WHERE status = $1 AND resolved_at < $2`, "Closed", cut)
		if err != nil {
			pm.log.Warn().Err(err).Str("component", "store").Msg("incident_lifecycle purge skipped or failed")
		} else {
			pm.log.Info().Str("component", "store").Int64("rows", tag.RowsAffected()).Msg("purged incident_lifecycle")
		}
	}

	return nil
}

func (pm *PartitionManager) analyzeRequestLogs(ctx context.Context) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	_, err := pm.db.Exec(ctx2, `ANALYZE request_logs`)
	return err
}
