// Package slogsyslog provides a slog sink for logging to syslog.
package slogsyslog

import (
	"context"
	"github.com/hlandau/slogkit/slogsyslog/syslog"
	"github.com/hlandau/slogkit/slogwriter"
	"golang.org/x/exp/slog"
)

type syslogHandler struct {
	cfg        Config
	l          *syslog.Logger
	underlying slog.Handler
}

// Configuration for the syslog logger.
type Config struct {
	// Handler options. Note that WriterFunc is overridden by this package.
	HandlerOptions slogwriter.HandlerOptions

	// Facility to log to.
	Facility syslog.Facility
}

// Returns a new slog.Handler which logs to the given syslog.Logger.
func New(l *syslog.Logger, cfg Config) slog.Handler {
	cfg.HandlerOptions.NoColor = true
	cfg.HandlerOptions.WriterFunc = func(ctx context.Context, b []byte, r slog.Record) error {
		return l.Write(ctx, syslog.Message{
			Time:     r.Time,
			Severity: mapLevelToSeverity(r.Level),
			Facility: cfg.Facility,
			ID:       r.Message,
			Body:     string(b),
		})
	}
	return slogwriter.NewJSONHandler(nil, &cfg.HandlerOptions)
}

func mapLevelToSeverity(level slog.Level) syslog.Severity {
	switch {
	case level <= slog.LevelDebug:
		return syslog.SeverityDebug
	case level <= slog.LevelInfo:
		return syslog.SeverityInfo
	case level <= 2:
		return syslog.SeverityNotice
	case level <= slog.LevelWarn:
		return syslog.SeverityWarning
	case level <= slog.LevelError:
		return syslog.SeverityErr
	case level <= 12:
		return syslog.SeverityCrit
	case level <= 16:
		return syslog.SeverityAlert
	default:
		return syslog.SeverityEmerg
	}
}
