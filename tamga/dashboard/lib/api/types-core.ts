/** One pool entry from GET /api/v1/health/detailed when policy defines providers.pools */
export interface DashboardHealthProviderPool {
  pool: string;
  healthy_count: number;
  total_count: number;
  providers: Array<{
    name: string;
    state: string;
    success_rate_observed?: number;
    p95_latency_ms?: number;
    requests_in_window?: number;
    last_failure?: string;
    failure_reason?: string;
  }>;
}

export interface DashboardHealthDetailed {
  proxy: "up" | string;
  proxy_status?: { up?: boolean };
  database: "connected" | "disconnected" | "not_configured" | string;
  scanner_count: number;
  uptime_seconds: number;
  policy_path: string;
  providers?: DashboardHealthProviderPool[];
  scan_latency_ms_p50?: number;
  scan_latency_ms_p95?: number;
  scan_latency_ms_p99?: number;
}

export interface DashboardHealthDetail extends DashboardHealthDetailed {
  version?: string;
  policy_name?: string;
  tls_enabled: boolean;
  mtls_enabled: boolean;
  redis_enabled: boolean;
  timestamp?: string;
  /** Base URL for Jaeger / Tempo UI — open trace by ID (from X-Tamga-Trace-Id). */
  trace_ui_url?: string;
  retention_enabled?: boolean;
  retention_last_run?: string;
}

export interface DashboardStatsV2 {
  total_requests: number;
  blocked_requests: number;
  redacted_requests: number;
  warned_requests: number;
  passed_requests: number;
  top_providers: Record<string, number>;
  top_finding_types: Record<string, number>;
  top_categories: Record<string, number>;
  uptime?: string;
  uptime_seconds?: number;
  scanner_latency_avg_ms: number;
  avg_input_risk_pct?: number;
}

export interface RiskScore {
  score: number;
  percentage: number;
  level: string;
  breakdown: Record<string, number>;
}

export interface SecurityEventConfidenceScore {
  total: number;
  action: string;
  reasoning: string;
  breakdown?: {
    format?: number;
    algorithm?: number;
    database?: number;
    context?: number;
  };
}

export interface SecurityFinding {
  type: string;
  severity: string;
  match: string;
  category: string;
  start_pos: number;
  end_pos: number;
  confidence: number;
  confidence_score?: SecurityEventConfidenceScore;
  action_taken?: string;
  metadata?: Record<string, string>;
  scanner_version?: string;
  dataset_version?: string;
}

export interface SecurityEvent {
  request_id: string;
  provider?: string;
  model?: string;
  event_type: string;
  action?: string;
  findings: SecurityFinding[];
  findings_count: number;
  endpoint?: string;
  scan_latency_ms?: number;
  total_latency_ms?: number;
  content_type?: string;
  timestamp: string;
  input_risk_pct?: number;
  risk_level?: string;
  input_risk?: RiskScore | null;
  output_risk?: RiskScore | null;
}

/** Detail response from GET /api/v1/events/{request_id} */
export interface SecurityEventDetail {
  request_id: string;
  timestamp: string;
  provider?: string;
  model?: string;
  action?: string;
  event_type?: string;
  input_risk: RiskScore;
  output_risk: RiskScore;
  findings: Array<{
    type: string;
    category: string;
    severity: string;
    match: string;
    confidence: number;
    action_taken: string;
    position: { start: number; end: number };
  }>;
  scan_latency_ms: number;
  total_latency_ms: number;
  policy_name: string;
  policy_version: string;
  input_tokens?: number;
  output_tokens?: number;
  endpoint?: string;
}

/** Active policy documents (currently single-item array). */
export type TamgaPolicy = Record<string, unknown>;

export interface CustomEntity {
  name: string;
  pattern: string;
  description?: string;
  severity: "critical" | "high" | "medium" | "low";
  action: string;
  confidence?: number;
}





export interface TimeseriesPoint {
  t: string;
  total: number;
  blocked: number;
  redacted: number;
  warned: number;
  scan_p95: number;
}

export interface TimeseriesResponse {
  range: string;
  bucket: string;
  points: TimeseriesPoint[];
}

export interface BreakdownResponse {
  range: string;
  by_type: Record<string, number>;
  by_category: Record<string, number>;
  by_severity: Record<string, number>;
  type_by_category: Record<string, Record<string, number>>;
}

export interface ModelStatsResponse {
  range: string;
  by_model: Record<string, number>;
  by_family: Record<string, number>;
}

export interface MTTRStats {
  overall_mttr_minutes: number;
  by_severity: Record<string, number>;
  trend: "improving" | "stable" | "worsening";
  sla_compliance: number;
}

export interface IncidentComment {
  author: string;
  text: string;
  created_at: string;
}

export type IncidentStatus = "Open" | "In Progress" | "Closed" | "False Positive";

export interface IncidentState {
  request_id: string;
  status: IncidentStatus;
  assignee?: string;
  reason?: string;
  tags?: string[];
  comments?: IncidentComment[];
  updated_at: string;
  created_at: string;
}

export interface IncidentPatch {
  status?: IncidentStatus;
  assignee?: string;
  reason?: string;
  tags?: string[];
  add_comment?: { author: string; text: string };
}
