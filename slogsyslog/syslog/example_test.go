package syslog_test

import (
	"context"
	"github.com/hlandau/slogkit/slogsyslog/syslog"
)

func Example() {
	log, err := syslog.New(syslog.Config{
		// By default, we connect to the local syslog daemon via a UNIX domain socket.
		//
		// Uncomment the below to use a UDP destination.
		//Address: "192.0.2.1",
	})
	if err != nil {
		// Handle error
	}

	log.Write(context.Background(), syslog.Message{
		Severity: syslog.SeverityDebug,
		Facility: syslog.FacilityDaemon,
		Body:     "This is a syslog message.",
	})
}
