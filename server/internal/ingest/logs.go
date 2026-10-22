package ingest

import (
	"context"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// LogWriter saves logs into the two log collections. The MongoDB Store
// satisfies it; tests use a fake.
type LogWriter interface {
	InsertErrorLogs(ctx context.Context, logs []model.LogRecord) error
	InsertAppLogs(ctx context.Context, logs []model.LogRecord) error
}

// WriteLogs returns a Write function for a log queue. It routes each batch
// by severity: ERROR and CRITICAL go to ErrorLogs marked PENDING for
// diagnosis; INFO and WARN go to AppLogs.
//
// If one group fails, the queue retries the whole batch. That is safe
// because the store ignores logs that were already saved (same _id).
func WriteLogs(w LogWriter) func(context.Context, []model.LogRecord) error {
	return func(ctx context.Context, logs []model.LogRecord) error {
		var errorLogs, appLogs []model.LogRecord
		for _, l := range logs {
			if model.IsErrorSeverity(l.Severity) {
				l.DiagnosisStatus = model.DiagnosisPending
				errorLogs = append(errorLogs, l)
			} else {
				appLogs = append(appLogs, l)
			}
		}
		if len(errorLogs) > 0 {
			if err := w.InsertErrorLogs(ctx, errorLogs); err != nil {
				return err
			}
		}
		if len(appLogs) > 0 {
			return w.InsertAppLogs(ctx, appLogs)
		}
		return nil
	}
}
