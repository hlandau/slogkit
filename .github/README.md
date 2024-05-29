# slogkit

[![Go Documentation](https://godocs.io/github.com/hlandau/slogkit?status.svg)](https://godocs.io/github.com/hlandau/slogkit)

A collection of utilities for Go's standard [slog](https://godocs.io/log/slog)
structured logging package.

The utilities include:

- slogdispatch, comprising:
    - a multi handler which implements `slog.Handler` and dispatches to multiple subhandlers
    - a contextual handler which implements `slog.Handler` and can dispatch based on a `context.Context`
    - a router handler which implements `slog.Handler` and can dispatch based on a set of arbitrary predicate rules
    - a default handler which implements `slog.Handler` and dispatches to `slog.Default()`
- sloghttp, which provides logging for `net/http` HTTP requests.
- slogsyslog, which provides a syslog sink for slog.
- slogsyslog/syslog, a syslog protocol implementation.
- slogtree, which allows slog loggers to be managed in a logical static hierarchy based on the package hierarchy.
- slogtreecfg, which allows a slogtree logging tree to be configured with common sinks easily.
- slogwriter, which provides prettier (e.g. colourised) output for slog.

See [slogtreecfg](https://godocs.io/github.com/hlandau/slogkit/slogtreecfg)'s documentation for a usage example.
