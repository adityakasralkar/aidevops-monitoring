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

	// DiagnosisStatus is set only for ErrorLogs.
	DiagnosisStatus string `bson:"diagnosis_status,omitempty"`
}
