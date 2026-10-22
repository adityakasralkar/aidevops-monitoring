package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef" // 32 characters

// env returns a lookup function backed by a map, standing in for os.LookupEnv.
func env(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestLoadValid(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want Config
	}{
		{
			name: "applies defaults when only the admin secret is set",
			vars: map[string]string{"ADMIN_SECRET": testSecret},
			want: Config{
				AppEnv:          EnvDevelopment,
				Port:            8080,
				LogLevel:        slog.LevelInfo,
				MongoURI:        "mongodb://localhost:27017",
				MongoDBName:     "aidevops",
				AdminSecret:     testSecret,
				IngestQueueSize: 5000,
				IngestWorkers:   4,
				ShutdownTimeout: 15 * time.Second,
			},
		},
		{
			name: "reads every variable",
			vars: map[string]string{
				"APP_ENV":           "production",
				"PORT":              "9090",
				"LOG_LEVEL":         "debug",
				"MONGO_URI":         "mongodb+srv://user:pass@cluster.example.net",
				"MONGO_DB_NAME":     "aidevops_prod",
				"ADMIN_SECRET":      testSecret,
				"INGEST_QUEUE_SIZE": "10000",
				"INGEST_WORKERS":    "8",
				"SHUTDOWN_TIMEOUT":  "30s",
			},
			want: Config{
				AppEnv:          EnvProduction,
				Port:            9090,
				LogLevel:        slog.LevelDebug,
				MongoURI:        "mongodb+srv://user:pass@cluster.example.net",
				MongoDBName:     "aidevops_prod",
				AdminSecret:     testSecret,
				IngestQueueSize: 10000,
				IngestWorkers:   8,
				ShutdownTimeout: 30 * time.Second,
			},
		},
		{
			name: "trims whitespace and treats blank values as unset",
			vars: map[string]string{
				"ADMIN_SECRET": "  " + testSecret + "\n",
				"PORT":         " 3000 ",
				"LOG_LEVEL":    "WARN",
				"APP_ENV":      "   ",
			},
			want: Config{
				AppEnv:          EnvDevelopment,
				Port:            3000,
				LogLevel:        slog.LevelWarn,
				MongoURI:        "mongodb://localhost:27017",
				MongoDBName:     "aidevops",
				AdminSecret:     testSecret,
				IngestQueueSize: 5000,
				IngestWorkers:   4,
				ShutdownTimeout: 15 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := load(env(tt.vars))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		wantErr string
	}{
		{"rejects a missing admin secret", map[string]string{}, "ADMIN_SECRET: is required"},
		{"rejects a short admin secret", map[string]string{"ADMIN_SECRET": "short"}, "ADMIN_SECRET: must be at least 32"},
		{"rejects an unknown app env", map[string]string{"APP_ENV": "staging"}, "APP_ENV: must be one of"},
		{"rejects a non-numeric port", map[string]string{"PORT": "abc"}, "PORT: must be an integer"},
		{"rejects port zero", map[string]string{"PORT": "0"}, "PORT: must be an integer between 1 and 65535"},
		{"rejects a port above 65535", map[string]string{"PORT": "70000"}, "PORT: must be an integer between 1 and 65535"},
		{"rejects an unknown log level", map[string]string{"LOG_LEVEL": "verbose"}, "LOG_LEVEL: must be one of"},
		{"rejects a non-mongo URI", map[string]string{"MONGO_URI": "http://localhost:27017"}, "MONGO_URI: must start with"},
		{"rejects a db name with a dot", map[string]string{"MONGO_DB_NAME": "aid.ops"}, "MONGO_DB_NAME: must not contain"},
		{"rejects a db name over 63 bytes", map[string]string{"MONGO_DB_NAME": strings.Repeat("a", 64)}, "MONGO_DB_NAME: must be at most 63"},
		{"rejects a zero queue size", map[string]string{"INGEST_QUEUE_SIZE": "0"}, "INGEST_QUEUE_SIZE: must be an integer"},
		{"rejects a queue smaller than one batch", map[string]string{"INGEST_QUEUE_SIZE": "499"}, "INGEST_QUEUE_SIZE: must be an integer between 500"},
		{"rejects zero workers", map[string]string{"INGEST_WORKERS": "0"}, "INGEST_WORKERS: must be an integer"},
		{"rejects a timeout without a unit", map[string]string{"SHUTDOWN_TIMEOUT": "15"}, "SHUTDOWN_TIMEOUT: must be a duration"},
		{"rejects a negative timeout", map[string]string{"SHUTDOWN_TIMEOUT": "-1s"}, "SHUTDOWN_TIMEOUT: must be a duration"},
		{"rejects a timeout above 5 minutes", map[string]string{"SHUTDOWN_TIMEOUT": "10m"}, "SHUTDOWN_TIMEOUT: must be a duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Supply a valid secret unless the case is about the secret itself.
			if !strings.HasPrefix(tt.wantErr, "ADMIN_SECRET") {
				tt.vars["ADMIN_SECRET"] = testSecret
			}
			_, err := load(env(tt.vars))
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadReportsAllErrorsTogether(t *testing.T) {
	_, err := load(env(map[string]string{"PORT": "abc", "LOG_LEVEL": "loud"}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	for _, key := range []string{"PORT", "LOG_LEVEL", "ADMIN_SECRET"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

func TestSecretsNeverLeak(t *testing.T) {
	const uriPassword = "s3cr3tpw"
	cfg, err := load(env(map[string]string{
		"ADMIN_SECRET": testSecret,
		"MONGO_URI":    "mongodb+srv://user:" + uriPassword + "@cluster.example.net",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var logBuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&logBuf, nil)).Info("config", "config", cfg)

	outputs := map[string]string{
		"fmt %v":  fmt.Sprintf("%v", cfg),
		"fmt %+v": fmt.Sprintf("%+v", cfg),
		"fmt %#v": fmt.Sprintf("%#v", cfg),
		"slog":    logBuf.String(),
	}
	for name, out := range outputs {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(out, testSecret) {
				t.Errorf("admin secret leaked: %s", out)
			}
			if !strings.Contains(out, redacted) {
				t.Errorf("expected %s in output: %s", redacted, out)
			}
		})
	}
	if strings.Contains(logBuf.String(), uriPassword) {
		t.Errorf("mongo password leaked in log: %s", logBuf.String())
	}
}

func TestErrorsNeverEchoSecrets(t *testing.T) {
	const badURI = "postgres://user:topsecret@db"
	_, err := load(env(map[string]string{"ADMIN_SECRET": "tooshort-secret", "MONGO_URI": badURI}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	for _, s := range []string{"tooshort-secret", "topsecret"} {
		if strings.Contains(err.Error(), s) {
			t.Errorf("error leaked %q: %v", s, err)
		}
	}
}
