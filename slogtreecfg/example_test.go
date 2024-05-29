package slogtreecfg_test

import (
	"context"

	"github.com/hlandau/slogkit/slogtree"
	"github.com/hlandau/slogkit/slogtreecfg"
)

var log, Log = slogtree.NewFacility("example")

var (
	knFoo = log.MakeKnownDebug("FOO")
)

func Example() {
	ctx := context.Background()
	ctx = slogtreecfg.InitConfig(ctx, slogtreecfg.Config{
		LogFile:         "/var/log/some.log",
		LogFileSeverity: "debug",
		LogFileFormat:   slogtreecfg.OutputFormatJSON,

		Stderr:         true,
		StderrSeverity: "debug",
	})

	log.LogCtx(ctx, knFoo, "param1", "value1")
}
