// Package model defines the data shapes shared by the other packages.
// It imports no other internal package.
package model

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Errors returned by the store and understood by the API layer.
var (
	ErrNotFound      = errors.New("not found")
	ErrEmailTaken    = errors.New("email already registered")
	ErrProjectExists = errors.New("project already exists")
)

// User is an account that can log in and own projects.
type User struct {
	ID           bson.ObjectID `bson:"_id,omitempty"`
	Email        string        `bson:"email"`
	PasswordHash string        `bson:"password_hash"`
	CreatedAt    time.Time     `bson:"created_at"`
}

// Session is one logged-in device or client. The token itself is never
// stored, only its hash.
type Session struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	UserID    bson.ObjectID `bson:"user_id"`
	TokenHash string        `bson:"token_hash"`
	CreatedAt time.Time     `bson:"created_at"`
	ExpiresAt time.Time     `bson:"expires_at"`
}

// Project is a registered application that sends telemetry with its API key.
// It is stored in the Applications collection.
type Project struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	OwnerID     bson.ObjectID `bson:"owner_id"`
	Name        string        `bson:"project_name"`
	Environment string        `bson:"environment"`
	APIKeyHash  string        `bson:"api_key_hash"`
	KeyPrefix   string        `bson:"key_prefix"`
	CreatedAt   time.Time     `bson:"created_at"`

	// KeyCreatedAt is when the current API key was made (creation or the
	// last rotation). LastUsedAt is when data last arrived with it,
	// updated at most once a minute. Both are nil for older documents.
	KeyCreatedAt *time.Time `bson:"key_created_at,omitempty"`
	LastUsedAt   *time.Time `bson:"last_used_at,omitempty"`
}

// Log severities accepted at ingestion.
const (
	SeverityInfo     = "INFO"
	SeverityWarn     = "WARN"
	SeverityError    = "ERROR"
	SeverityCritical = "CRITICAL"
)

// Diagnosis status values for ErrorLogs. Every new error starts as PENDING;
// the AI layer (later phase) moves it on.
const DiagnosisPending = "PENDING"

// IsErrorSeverity reports whether a log of this severity belongs in
// ErrorLogs (ERROR, CRITICAL) rather than AppLogs (INFO, WARN).
func IsErrorSeverity(severity string) bool {
	return severity == SeverityError || severity == SeverityCritical
}

// LogRecord is one log line, stored in ErrorLogs or AppLogs.
type LogRecord struct {
	ID          bson.ObjectID  `bson:"_id"`
	ProjectID   bson.ObjectID  `bson:"project_id"`
	ServiceName string         `bson:"service_name"`
	Severity    string         `bson:"severity"`
	Message     string         `bson:"message"`
	StackTrace  string         `bson:"stack_trace,omitempty"`
	Attributes  map[string]any `bson:"attributes,omitempty"`
	Timestamp   time.Time      `bson:"timestamp"`   // when the event happened (from the sender)
	ReceivedAt  time.Time      `bson:"received_at"` // when the server accepted it

	// DiagnosisStatus is set only for ErrorLogs. The AI fields are filled
	// in later by the AI diagnosis layer.
	DiagnosisStatus string `bson:"diagnosis_status,omitempty"`
	AIRootCause     string `bson:"ai_root_cause,omitempty"`
	AISuggestedFix  string `bson:"ai_suggested_fix,omitempty"`
}

// Metric types (Decision #13).
const (
	MetricGauge     = "gauge"     // a value right now, e.g. CPU 73%
	MetricSum       = "sum"       // a running total, e.g. 1532 requests
	MetricHistogram = "histogram" // a distribution, e.g. response times in buckets
)

// Metric temporality values.
const (
	TemporalityCumulative = "cumulative"
	TemporalityDelta      = "delta"
)

// MetricPoint is one metric measurement, stored in TelemetryMetrics.
// Gauge and sum use Value; histograms use Count, Sum, Min, Max and the
// buckets. Pointers distinguish "not sent" from a real zero.
type MetricPoint struct {
	ID          bson.ObjectID `bson:"_id"`
	ProjectID   bson.ObjectID `bson:"project_id"`
	ServiceName string        `bson:"service_name"`
	MetricName  string        `bson:"metric_name"`
	MetricType  string        `bson:"metric_type"`
	Unit        string        `bson:"unit,omitempty"`

	// Temporality says how sums and histograms count, as OpenTelemetry
	// reports it: "cumulative" (a running total since the app started) or
	// "delta" (only this interval). Empty when unknown (custom JSON).
	// Monotonic is set for OpenTelemetry sums: true means the total only
	// grows (a counter), false means it can go down (an up/down counter).
	Temporality string `bson:"temporality,omitempty"`
	Monotonic   *bool  `bson:"monotonic,omitempty"`

	Value *float64 `bson:"value,omitempty"`

	Count        *int64    `bson:"count,omitempty"`
	Sum          *float64  `bson:"sum,omitempty"`
	Min          *float64  `bson:"min,omitempty"`
	Max          *float64  `bson:"max,omitempty"`
	BucketBounds []float64 `bson:"bucket_bounds,omitempty"` // upper bounds, increasing
	BucketCounts []int64   `bson:"bucket_counts,omitempty"` // one more than BucketBounds

	Attributes map[string]any `bson:"attributes,omitempty"`
	Timestamp  time.Time      `bson:"timestamp"`
	ReceivedAt time.Time      `bson:"received_at"`

	// StartTime is when a cumulative series began counting (usually when
	// the app started), as OpenTelemetry reports it. A new start time
	// means the totals restarted from zero.
	StartTime *time.Time `bson:"start_time,omitempty"`
}

// ServiceInfo summarises one service (program) seen in a project's data.
// Services are never registered; they appear when their data arrives.
type ServiceInfo struct {
	Name     string
	Language string // from OpenTelemetry's telemetry.sdk.language, if sent
	LastSeen time.Time
	Logs     int64 // logs of every level in the time range
	Errors   int64 // ERROR and CRITICAL logs in the time range
	Metrics  int64 // metric points in the time range
}

// MetricInfo summarises one metric of one service in a time range.
type MetricInfo struct {
	Name      string
	Service   string
	Type      string
	Unit      string
	LastSeen  time.Time
	LastValue *float64 // latest value of a gauge or sum
	Points    int64
}

// LogQuery selects a page of logs, newest first.
type LogQuery struct {
	ProjectID       bson.ObjectID
	Severities      []string // empty = all
	Service         string   // empty = all
	Search          string   // case-insensitive text in the message
	DiagnosisStatus string   // only for errors; empty = any
	Since, Until    time.Time
	// Before continues a previous page: only logs older than this
	// (timestamp, id) pair are returned.
	BeforeTime time.Time
	BeforeID   bson.ObjectID
	Limit      int
}

// MetricQuery selects metric points in a time range, oldest first.
type MetricQuery struct {
	ProjectID    bson.ObjectID
	Names        []string // empty = all
	Service      string   // empty = all
	Since, Until time.Time
	Limit        int
}
