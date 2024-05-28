// Package sloghttp provides a simple logging solution to wrap HTTP handlers.
package sloghttp

import (
	"net/http"
	"runtime"

	"github.com/hlandau/slogkit/slogtree"
)

var log, Log = slogtree.NewFacility("sloghttp")

var (
	knHttpReqStart  = log.MakeKnownInfo("HTTP_REQ_START", "desc", "HTTP request has started")
	knHttpReqFinish = log.MakeKnownInfo("HTTP_REQ_FINISH", "desc", "HTTP request has finished")
	knHttpReqPanic  = log.MakeKnownError("HTTP_REQ_PANIC", "desc", "panic during handling of HTTP request")
)

type logHandler struct {
	underlying http.Handler
}

func (lh logHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	log.LogCtx(req.Context(), knHttpReqStart, "method", req.Method, "url", req.URL.String(), "host", req.Host, "proto", req.Proto, "raddr", req.RemoteAddr, "userAgent", req.Header.Get("User-Agent"), "referer", req.Header.Get("Referer"))

	defer func() {
		if r := recover(); r != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]

			log.LogCtx(req.Context(), knHttpReqPanic, "error", r, "stack", string(buf))
			panic(r)
		} else {
			log.LogCtx(req.Context(), knHttpReqFinish)
		}
	}()

	lh.underlying.ServeHTTP(rw, req)
}

// Returns an HTTP handler which wraps the given handler and logs request
// events.
func LogHandler(h http.Handler) http.Handler {
	return logHandler{h}
}
