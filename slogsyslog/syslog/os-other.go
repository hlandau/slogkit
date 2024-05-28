//go:build !unix
// +build !unix

package syslog

func determineOSSpecificConnTargets(network, address string) ([]connTarget, error) {
	return nil, nil
}
