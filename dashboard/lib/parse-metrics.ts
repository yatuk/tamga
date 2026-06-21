/**
 * Parse Prometheus text format into a typed record.
 *
 * The Go proxy serves metrics as plain text at /api/v1/metrics.
 * This utility extracts numeric values for dashboard display without
 * adding a Prometheus client library dependency.
 */
export interface PoolMetrics {
  workersIdle: number;
  workersActive: number;
  queueDepth: number;
  queueSize: number;
  utilization: number;
  jobsSubmitted: number;
  jobsCompleted: number;
  jobsFailed: number;
  jobsShed: number;
  perScannerDurationMs: Record<string, number>;
}

/** Parse a Prometheus text response into PoolMetrics. Unknown keys are ignored. */
export function parsePoolMetrics(text: string): PoolMetrics {
  const out: PoolMetrics = {
    workersIdle: 0,
    workersActive: 0,
    queueDepth: 0,
    queueSize: 0,
    utilization: 0,
    jobsSubmitted: 0,
    jobsCompleted: 0,
    jobsFailed: 0,
    jobsShed: 0,
    perScannerDurationMs: {},
  };

  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;

    // Match: metric_name{labels} value
    const m = trimmed.match(/^(\w+)(?:\{([^}]*)\})?\s+(\S+)/);
    if (!m) continue;

    const [, name, labels, val] = m;
    const num = Number(val);
    if (isNaN(num)) continue;

    switch (name) {
      case "tamga_scanner_pool_workers": {
        if (labels?.includes('state="idle"')) out.workersIdle = num;
        if (labels?.includes('state="active"')) out.workersActive = num;
        break;
      }
      case "tamga_scanner_pool_queue_depth":
        out.queueDepth = num;
        break;
      case "tamga_scanner_pool_queue_size":
        out.queueSize = num;
        break;
      case "tamga_scanner_pool_utilization":
        out.utilization = num;
        break;
      case "tamga_scanner_pool_jobs_total": {
        if (labels?.includes('status="submitted"')) out.jobsSubmitted = num;
        if (labels?.includes('status="completed"')) out.jobsCompleted = num;
        if (labels?.includes('status="failed"')) out.jobsFailed = num;
        break;
      }
      case "tamga_scanner_pool_jobs_shed_total":
        out.jobsShed = num;
        break;
      case "tamga_scanner_job_duration_ms_avg": {
        const scannerMatch = labels?.match(/scanner="(\w+)"/);
        if (scannerMatch) out.perScannerDurationMs[scannerMatch[1]] = num;
        break;
      }
    }
  }

  return out;
}
