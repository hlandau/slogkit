// Package slogtreecfg provides a convenient means of realising commonly
// desired slogtree configuration for use in an application.
//
// This package is intended for top-level use by an application's main
// function, not by libraries, as libraries should not dictate how
// their logging is configured.
package slogtreecfg

import (
	"context"

	"github.com/hlandau/slogkit/slogdispatch"
	"github.com/hlandau/slogkit/slogtree"
	"golang.org/x/exp/slog"
)

// Determines the output format to use when logging to a sink.
type OutputFormat string

const (
	// Use the default log output format.
	OutputFormatDefault OutputFormat = ""

	// Use the textual log output format.
	OutputFormatText = "text"

	// Use the JSON (really, JSONL) log output format.
	OutputFormatJSON = "json"
)

// Configuration settings which determine how slogtree-based logging is setup.
type Config struct {
	Severity string `help:"Syslog log severity to act as global filter (optional)"`

	// If non-empty, this is the path to a file to which log entries should be
	// written, one per line. The file is created if it does not exist and is
	// truncated if it already exists.
	LogFile string `help:"Path to file to log to"`

	// A severity name which describes the minimum severity which should be
	// logged to the log file specified in LogFile. If empty, no specific
	// severity filtering is applied to the file log sink.
	LogFileSeverity string `help:"Log severity filter for log file output"`

	// The output format to use when logging to the file specified in LogFile.
	LogFileFormat OutputFormat `help:"Output format for log file ('text' or 'json')"`

	// If true, log to os.Stderr.
	Stderr bool `help:"Log to stderr"`

	// A severity name which describes the minimum severity which should be
	// logged to os.Stderr. If empty, no specific severity filtering is applied
	// to the stderr log sink.
	StderrSeverity string `help:"Log severity filter for stderr output"`

	// The output format to use when logging to os.Stderr.
	StderrFormat OutputFormat `help:"Output format for log file ('text' or 'json')"`

	// If true, log to syslog.
	Syslog bool `help:"Log to syslog"`

	// A string of the form "unixgram:/foo" or "udp:127.0.0.1:514" or similar. If
	// empty, defaults to using the local system syslog server via a UNIX domain
	// socket.
	SyslogTarget string `help:"Syslog target address (unixgram:/foo, udp:127.0.0.1:514, default is local system)"`

	// A severity name which describes the minimum severity which should be
	// logged to syslog. If empty, no specific severity filtering is applied to
	// the stderr log sink.
	SyslogSeverity string `help:"Log severity filter for syslog output"`

	// Syslog facility to log to.
	SyslogFacility string `help:"Syslog facility to log to"`
}

var flushables []func()

var log, Log = slogtree.NewFacility("slogtreecfg")

var (
	knSinkInitError = log.MakeKnownError("LOG_SINK_INIT_FAIL", "desc", "Failed to initialise a log output sink")
)

// Call once to initialise logging configuration in an application.
//
// ctx should usually be context.Background(), but you may use another context.
// The returned context wraps the provided context and provides contextual
// logging configuration.
func InitConfig(ctx context.Context, cfg Config) context.Context {
	// Use contextual handler lookup, with a simple contextual resolver which
	// falls back to the slog default handler.
	sr := slogdispatch.NewSimpleResolver(slogdispatch.NewDefaultHandler())
	slogtree.Root().SetHandler(slogdispatch.NewContextualHandler(sr))

	sinks, initErrors := initConfig(cfg)

	// Multi-dispatch handler which writes log entries to all of our sinks.
	// Set it as the default.
	slog.SetDefault(slog.New(slogdispatch.NewMultiHandler(sinks)))

	// Prime a context with empty state so we can use WithAttrs.
	rootCtx := sr.WithAttrs(ctx)

	for _, initError := range initErrors {
		log.LogCtx(rootCtx, knSinkInitError, "error", initError)
	}

	return rootCtx
}

// Actual initialisation of all configured sinks. Any errors which occur during
// initialisation of one or more sinks are returned in errors.
func initConfig(cfg Config) (sinks []slog.Handler, errors []error) {
	for _, f := range []func(cfg Config) (slog.Handler, error){
		setupLogFile,
		setupStderr,
		setupSyslog,
	} {
		h, err := f(cfg)
		if err != nil {
			errors = append(errors, err)
		} else if h != nil {
			sinks = append(sinks, h)
		}
	}

	return
}

// Flush all flushables.
func Flush() {
	for _, f := range flushables {
		f()
	}
}
