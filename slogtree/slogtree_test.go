package slogtree_test

import (
	"context"
	"time"

	"github.com/hlandau/slogkit/slogdispatch"
	"github.com/hlandau/slogkit/slogtree"
)

// Declare a log site for this package.
var log, Log = slogtree.NewFacility("foo/bar/baz")

var (
	// Declare known log message types with severities ERROR and INFO respectively.
	//
	// An item of metadata "desc" is attached to the REQ_FAILED type.
	knReqFailed = log.MakeKnownError("REQ_FAILED", "desc", "The request failed.")
	knReqOK     = log.MakeKnownInfo("REQ_OK")
)

func doStuff() error {
	return nil // placeholder
}

func Example() {
	ctx := context.Background() // placeholder

	if err := doStuff(); err != nil {
		// Log the REQ_FAILED message type with an extra structured attribute.
		//
		// Standard style:
		log.LogCtx(ctx, knReqFailed, "error", err)

		// If you need to add fields using the context, use slogdispatch:
		ctx2 := slogdispatch.WithAttrs(ctx, "fruitType", "orange")
		log.LogCtx(ctx2, knReqOK, "duration", 42*time.Millisecond)
	}
}
