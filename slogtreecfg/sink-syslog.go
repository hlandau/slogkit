package slogtreecfg

import (
	"errors"
	"fmt"
	"github.com/hlandau/slogkit/slogsyslog"
	"github.com/hlandau/slogkit/slogsyslog/syslog"
	"github.com/hlandau/slogkit/slogwriter"
	"golang.org/x/exp/slog"
	"gopkg.in/hlandau/svcutils.v1/exepath"
	"strings"
)

func setupSyslog(cfg Config) (slog.Handler, error) {
	if !cfg.Syslog {
		return nil, nil
	}

	facility, err := syslog.ParseFacility(cfg.SyslogFacility)
	if err != nil {
		return nil, fmt.Errorf("cannot parse syslog facility name: %q: %v", cfg.SyslogFacility, err)
	}

	network, address, err := parseSyslogTarget(cfg.SyslogTarget)
	if err != nil {
		return nil, fmt.Errorf("cannot parse syslog target name: %q: %v", cfg.SyslogTarget, err)
	}

	scfg := syslog.Config{
		Network:  network,
		Address:  address,
		ProcName: exepath.ProgramName,
	}

	l, err := syslog.New(scfg)
	if err != nil {
		return nil, err
	}

	h := slogsyslog.New(l, slogsyslog.Config{
		HandlerOptions: slogwriter.HandlerOptions{
			Level: slog.LevelDebug,
		},
		Facility: facility,
	})
	return h, nil
}

func parseSyslogTarget(s string) (network, address string, err error) {
	if s == "" {
		return
	}

	l, r, ok := strings.Cut(s, ":")
	if !ok {
		err = errors.New("target must be of form 'network:address'")
		return
	}

	r = strings.TrimPrefix(r, "//")
	return l, r, nil
}
