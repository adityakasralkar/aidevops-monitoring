// Package config reads the server settings from environment variables.
//
// Settings are read once, when the server starts. If any value is wrong,
// all problems are reported together so they can be fixed in one go.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Allowed values for APP_ENV.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// minAdminSecretLen is the minimum length of ADMIN_SECRET.
// `openssl rand -hex 32` gives a 64-character secret.
const minAdminSecretLen = 32

// maxDBNameLen is the longest database name MongoDB allows (in bytes).
const maxDBNameLen = 63

// Secret holds a sensitive value such as a password.
// When printed or logged it shows [REDACTED] instead of the real value.
type Secret string

const redacted = "[REDACTED]"

// String hides the real value when printed with fmt (%s, %v).
func (Secret) String() string { return redacted }

// GoString hides the real value when printed with fmt %#v.
func (Secret) GoString() string { return redacted }

// LogValue hides the real value when written to the log.
func (Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// Config holds all server settings.
type Config struct {
	AppEnv          string
	Port            int
	LogLevel        slog.Level
	MongoURI        string
	MongoDBName     string
	AdminSecret     Secret
	IngestQueueSize int
	IngestWorkers   int
	ShutdownTimeout time.Duration
}

// Load reads the settings from the process environment variables.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

// load does the actual reading. The lookup function is passed in so that
// tests can supply their own values without changing real environment variables.
func load(lookup func(string) (string, bool)) (Config, error) {
	r := &reader{lookup: lookup}

	// Each setting: name, default value, then the allowed values or range.
	cfg := Config{
		AppEnv:          r.oneOf("APP_ENV", EnvDevelopment, EnvDevelopment, EnvProduction),
		Port:            r.intInRange("PORT", 8080, 1, 65535),
		LogLevel:        r.logLevel("LOG_LEVEL", slog.LevelInfo),
		MongoURI:        r.mongoURI("MONGO_URI", "mongodb://localhost:27017"),
		MongoDBName:     r.dbName("MONGO_DB_NAME", "aidevops"),
		AdminSecret:     r.secret("ADMIN_SECRET", minAdminSecretLen),
		IngestQueueSize: r.intInRange("INGEST_QUEUE_SIZE", 5000, 1, 1_000_000),
		IngestWorkers:   r.intInRange("INGEST_WORKERS", 4, 1, 256),
		ShutdownTimeout: r.durationInRange("SHUTDOWN_TIMEOUT", 15*time.Second, time.Second, 5*time.Minute),
	}

	// If any setting was wrong, return all the problems as one error.
	if err := errors.Join(r.errs...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return cfg, nil
}

// LogValue controls how Config appears in the log: the admin secret is
// hidden and any password inside the Mongo URI is masked.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("app_env", c.AppEnv),
		slog.Int("port", c.Port),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("mongo_uri", redactURI(c.MongoURI)),
		slog.String("mongo_db_name", c.MongoDBName),
		slog.Any("admin_secret", c.AdminSecret),
		slog.Int("ingest_queue_size", c.IngestQueueSize),
		slog.Int("ingest_workers", c.IngestWorkers),
		slog.Duration("shutdown_timeout", c.ShutdownTimeout),
	)
}

// redactURI replaces the password in a connection URI with "xxxxx".
func redactURI(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return redacted
	}
	return u.Redacted()
}

// reader reads environment variables one at a time and keeps a list of
// every error it finds, so they can all be reported at the end.
type reader struct {
	lookup func(string) (string, bool)
	errs   []error
}

// get returns the value of key with surrounding spaces removed.
// An empty value is treated the same as a missing one.
func (r *reader) get(key string) (string, bool) {
	v, ok := r.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

// fail records an error for key, e.g. "PORT: must be an integer ...".
func (r *reader) fail(key, format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf("%s: %s", key, fmt.Sprintf(format, args...)))
}

// oneOf returns the value of key if it is one of the allowed values.
// If key is not set, it returns def.
func (r *reader) oneOf(key, def string, allowed ...string) string {
	v, ok := r.get(key)
	if !ok {
		return def
	}
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	r.fail(key, "must be one of %s, got %q", strings.Join(allowed, ", "), v)
	return def
}

// intInRange returns key as a whole number between lo and hi.
// If key is not set, it returns def.
func (r *reader) intInRange(key string, def, lo, hi int) int {
	v, ok := r.get(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		r.fail(key, "must be an integer between %d and %d, got %q", lo, hi, v)
		return def
	}
	return n
}

// durationInRange returns key as a time duration (such as "15s") between
// lo and hi. If key is not set, it returns def.
func (r *reader) durationInRange(key string, def, lo, hi time.Duration) time.Duration {
	v, ok := r.get(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < lo || d > hi {
		r.fail(key, "must be a duration between %s and %s (e.g. 15s), got %q", lo, hi, v)
		return def
	}
	return d
}

// logLevel turns "debug", "info", "warn" or "error" (any letter case)
// into a log level. If key is not set, it returns def.
func (r *reader) logLevel(key string, def slog.Level) slog.Level {
	v, ok := r.get(key)
	if !ok {
		return def
	}
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	r.fail(key, "must be one of debug, info, warn, error, got %q", v)
	return def
}

// mongoURI only checks that the value starts with a MongoDB scheme; the
// driver checks the rest when it connects. The value is never included in
// the error because it may contain a password.
func (r *reader) mongoURI(key, def string) string {
	v, ok := r.get(key)
	if !ok {
		return def
	}
	if !strings.HasPrefix(v, "mongodb://") && !strings.HasPrefix(v, "mongodb+srv://") {
		r.fail(key, "must start with mongodb:// or mongodb+srv://")
		return def
	}
	return v
}

// dbName checks the value against MongoDB's rules for database names:
// not too long and no special characters.
func (r *reader) dbName(key, def string) string {
	v, ok := r.get(key)
	if !ok {
		return def
	}
	if len(v) > maxDBNameLen {
		r.fail(key, "must be at most %d bytes, got %d", maxDBNameLen, len(v))
		return def
	}
	if strings.ContainsAny(v, "/\\. \"$*<>:|?\x00") {
		r.fail(key, "must not contain any of / \\ . space \" $ * < > : | ? or NUL, got %q", v)
		return def
	}
	return v
}

// secret requires key to be set and at least minLen characters long.
// It has no default, and the value is never included in the error.
func (r *reader) secret(key string, minLen int) Secret {
	v, ok := r.get(key)
	if !ok {
		r.fail(key, "is required (generate one with: openssl rand -hex 32)")
		return ""
	}
	if len(v) < minLen {
		r.fail(key, "must be at least %d characters, got %d", minLen, len(v))
		return ""
	}
	return Secret(v)
}
