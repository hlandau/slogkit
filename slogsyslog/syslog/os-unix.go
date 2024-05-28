//go:build unix
// +build unix

package syslog

import "strings"

func determineOSSpecificConnTargets(network, address string) ([]connTarget, error) {
	if (network == "" || network == "unix" || network == "unixgram") &&
		(address == "" || strings.HasPrefix(address, "/")) {
		var targets []connTarget
		for _, n := range []string{"unixgram", "unix"} {
			if network != "" && n != network {
				continue
			}
			if address != "" {
				targets = append(targets, connTarget{n, address})
			} else {
				for _, p := range []string{"/dev/log", "/var/run/syslog", "/var/run/log"} {
					targets = append(targets, connTarget{n, p})
				}
			}
		}
		return targets, nil
	}

	return nil, nil
}
