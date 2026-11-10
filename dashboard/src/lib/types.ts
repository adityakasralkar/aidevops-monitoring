// API response types. They mirror the Go structs in server/internal/api.

/** GET /api/v1/auth/me and the `user` field of a login response. */
export interface User {
  id: string
  email: string
  created_at: string
}

/** POST /api/v1/auth/login */
export interface LoginResponse {
  token: string
  token_type: 'Bearer'
  expires_at: string
  user: User
}

/** Every error response: { "error": { "code": "...", "message": "..." } } */
export interface ErrorBody {
  error: { code: string; message: string }
}

export type Environment = 'live' | 'test'

/** A project as listed by GET /api/v1/projects and GET /api/v1/projects/{id}. */
export interface Project {
  id: string
  name: string
  environment: Environment
  key_prefix: string
  created_at: string
  key_created_at: string
  /** null until the first data arrives with this project's key. */
  last_used_at: string | null
}

/** POST /api/v1/projects: the project plus its API key, returned only this once. */
export interface CreatedProject extends Project {
  api_key: string
}

/** One program that sent data, from GET /api/v1/projects/{id}/services. */
export interface Service {
  name: string
  /** From the SDK, e.g. "nodejs", "python"; missing if the SDK didn't say. */
  language?: string
  last_seen: string
  logs: number
  errors: number
  metrics: number
}

export type Severity = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'CRITICAL'
export type DiagnosisStatus = 'PENDING' | 'COMPLETED' | 'FAILED'

/** One log line, from the logs and errors endpoints. */
export interface LogEntry {
  id: string
  timestamp: string
  received_at: string
  severity: Severity
  service: string
  message: string
  stack_trace?: string
  attributes?: Record<string, unknown>
  /** Only on ERROR / CRITICAL logs. */
  diagnosis_status?: DiagnosisStatus
  ai_root_cause?: string
  ai_suggested_fix?: string
}

export interface LogPage {
  range: string
  items: LogEntry[]
  /** Empty when there are no older entries. */
  next_cursor?: string
}

/** One time bucket of the overview charts. */
export interface OverviewStep {
  t: string
  requests: number
  /** Requests answered with a 5xx status. */
  errors: number
  p50_ms: number | null
  p95_ms: number | null
}

export interface OverviewService {
  name: string
  language?: string
  last_seen: string
  requests_per_min: number
  /** 0–1; null when the service had no requests. */
  error_rate: number | null
  p95_ms: number | null
  error_logs: number
}

/** GET /api/v1/projects/{id}/overview */
export interface Overview {
  range: string
  since: string
  until: string
  step_seconds: number
  last_data_at: string | null
  /** false: the app sends logs but no HTTP request metrics yet. */
  has_http_metrics: boolean
  requests: number
  requests_per_min: number
  /** 0–1, share of requests with a 5xx status. */
  error_rate: number | null
  p50_ms: number | null
  p95_ms: number | null
  /** ERROR / CRITICAL logs in the range. */
  errors: number
  /** Of those, how many wait for AI diagnosis. */
  errors_pending: number
  series: OverviewStep[]
  services: OverviewService[]
  truncated: boolean
}

/** One step of a metric chart: gauges and sums fill value, histograms count/p50/p95. */
export interface ChartPoint {
  t: string
  value?: number
  count?: number
  p50?: number
  p95?: number
}

/** One metric of one service as a chart. */
export interface MetricSeries {
  name: string
  service: string
  type: 'gauge' | 'sum' | 'histogram'
  unit?: string
  step_seconds: number
  points: ChartPoint[]
  distribution?: { bounds: number[]; counts: number[] }
}

/** GET /api/v1/projects/{id}/errors/{errorId}: the error and the 10 minutes before it. */
export interface ErrorDetail extends LogEntry {
  context: {
    window_start: string
    window_end: string
    metrics: MetricSeries[]
    related_logs: LogEntry[]
  }
}
