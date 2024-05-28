package slogtreecfg

import (
	"bufio"
	"fmt"
	"github.com/hlandau/slogkit/slogwriter"
	"github.com/mattn/go-isatty"
	"golang.org/x/exp/slog"
	"io"
	"os"
)

func handlerFromFile(f *os.File, format OutputFormat) (slog.Handler, error) {
	var w io.Writer = f
	shouldBuffer := !isatty.IsTerminal(f.Fd())

	if shouldBuffer {
		bw := bufio.NewWriter(f)
		w = bw
		flushables = append(flushables, func() {
			bw.Flush()
		})
	}

	if format == OutputFormatJSON {
		ho := &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}

		return slog.NewJSONHandler(w, ho), nil
	} else if format == OutputFormatDefault || format == OutputFormatText {
		ho := &slogwriter.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
			NoColor:   shouldInhibitColor,
		}

		return slogwriter.NewTextHandler(w, ho), nil
	} else {
		return nil, fmt.Errorf("invalid log file format: %q", format)
	}
}

func setupLogFile(cfg Config) (slog.Handler, error) {
	if cfg.LogFile == "" {
		return nil, nil
	}

	f, err := os.Open(cfg.LogFile)
	if err != nil {
		return nil, err
	}

	return handlerFromFile(f, cfg.LogFileFormat)
}

func setupStderr(cfg Config) (slog.Handler, error) {
	if !cfg.Stderr {
		return nil, nil
	}

	return handlerFromFile(os.Stderr, cfg.StderrFormat)
}

var shouldInhibitColor = determineShouldInhibitColor()

func determineShouldInhibitColor() bool {
	v := os.Getenv("NO_COLOR")
	if v == "force" {
		return false
	} else if len(v) > 0 {
		return true
	}

	if !isatty.IsTerminal(2) {
		return true
	}

	return false
}
