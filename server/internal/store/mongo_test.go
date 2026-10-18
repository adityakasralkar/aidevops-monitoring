package store

import (
	"context"
	"testing"
	"time"
)

func TestConnectFails(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"returns an error when the server is unreachable", "mongodb://127.0.0.1:1/?connectTimeoutMS=200"},
		{"returns an error when the URI is malformed", "mongodb://bad host:27017"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			start := time.Now()
			s, err := Connect(ctx, tt.uri, "aidevops_test")
			if err == nil {
				_ = s.Close(context.Background())
				t.Fatal("expected an error, got nil")
			}
			if elapsed := time.Since(start); elapsed > 3*time.Second {
				t.Errorf("took %s to fail; want it bounded by the context", elapsed)
			}
		})
	}
}
