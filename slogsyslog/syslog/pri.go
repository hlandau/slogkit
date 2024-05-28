package syslog

import (
	"errors"
	"strings"
)

// SYSLOG protocol severity value. The valid range is [0,7].
type Severity int

const (
	SeverityEmerg Severity = iota
	SeverityAlert
	SeverityCrit
	SeverityErr
	SeverityWarning
	SeverityNotice
	SeverityInfo
	SeverityDebug
)

var errBadSeverity = errors.New("bad severity string")

// Case-insensitively parses a string specifying a severity.
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(s) {
	case "emergency", "emerg":
		return SeverityEmerg, nil
	case "alert":
		return SeverityAlert, nil
	case "critical", "crit":
		return SeverityCrit, nil
	case "error", "err":
		return SeverityErr, nil
	case "warning", "warn":
		return SeverityWarning, nil
	case "notice":
		return SeverityNotice, nil
	case "info":
		return SeverityInfo, nil
	case "debug":
		return SeverityDebug, nil
	default:
		return SeverityDebug, errBadSeverity
	}
}

// SYSLOG protocol facility value. The valid range is [0,23].
type Facility int

const (
	FacilityKern Facility = iota
	FacilityUser
	FacilityMail
	FacilityDaemon
	FacilityAuth
	FacilitySyslog
	FacilityLpr
	FacilityNews
	FacilityUUCP
	FacilityCron
	FacilityAuthPriv
	FacilityFtp
	FacilityNtp      // Not universally supported
	FacilityLogAudit // Not universally supported
	FacilityLogAlert // Not universally supported
	FacilityClock    // Not universally supported
	FacilityLocal0
	FacilityLocal1
	FacilityLocal2
	FacilityLocal3
	FacilityLocal4
	FacilityLocal5
	FacilityLocal6
	FacilityLocal7
)

var errBadFacility = errors.New("bad facility string")

// Case-insensitively parses a string specifying a facility.
func ParseFacility(s string) (Facility, error) {
	switch strings.ToLower(s) {
	case "kern", "kernel":
		return FacilityKern, nil
	case "user":
		return FacilityUser, nil
	case "mail":
		return FacilityMail, nil
	case "daemon":
		return FacilityDaemon, nil
	case "auth":
		return FacilityAuth, nil
	case "syslog":
		return FacilitySyslog, nil
	case "lpr":
		return FacilityLpr, nil
	case "news":
		return FacilityNews, nil
	case "uucp":
		return FacilityUUCP, nil
	case "cron":
		return FacilityCron, nil
	case "authpriv":
		return FacilityAuthPriv, nil
	case "ftp":
		return FacilityFtp, nil
	case "ntp":
		return FacilityNtp, nil
	case "logaudit":
		return FacilityLogAudit, nil
	case "logalert":
		return FacilityLogAlert, nil
	case "clock":
		return FacilityClock, nil
	case "local0":
		return FacilityLocal0, nil
	case "local1":
		return FacilityLocal1, nil
	case "local2":
		return FacilityLocal2, nil
	case "local3":
		return FacilityLocal3, nil
	case "local4":
		return FacilityLocal4, nil
	case "local5":
		return FacilityLocal5, nil
	case "local6":
		return FacilityLocal6, nil
	case "local7":
		return FacilityLocal7, nil
	default:
		return FacilityLocal7, errBadFacility
	}
}
