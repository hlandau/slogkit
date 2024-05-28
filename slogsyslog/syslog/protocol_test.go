package syslog

import (
	"bytes"
	"testing"
	"time"
)

var encodingTests = []struct {
	Expected       string
	Protocol       Protocol
	Framing        Framing
	BOMMode        BOMMode
	Severity       Severity
	Facility       Facility
	HostName       string
	ProcName       string
	PID            int
	MessageID      string
	MessageBody    string
	StructuredData string
}{
	{"<36>1 2021-10-11T07:25:00Z HostName ProcName 12345 MsgID SD \ufeffMsgBody",
		ProtocolV1Net, FramingNone, BOMModeAlways,
		SeverityWarning, FacilityAuth, "HostName", "ProcName", 12345, "MsgID", "MsgBody", "SD"},
	{"<36>1 2021-10-11T07:25:00Z HostName ProcName 12345 MsgID SD \ufeffMsgBody\n",
		ProtocolV1Net, FramingDelimiterLF, BOMModeAlways,
		SeverityWarning, FacilityAuth, "HostName", "ProcName", 12345, "MsgID", "MsgBody", "SD"},
	{"<36>1 2021-10-11T07:25:00Z HostName ProcName 12345 MsgID SD \ufeffMsgBody\x00",
		ProtocolV1Net, FramingDelimiterNUL, BOMModeAlways,
		SeverityWarning, FacilityAuth, "HostName", "ProcName", 12345, "MsgID", "MsgBody", "SD"},
	{"70 <36>1 2021-10-11T07:25:00Z HostName ProcName 12345 MsgID SD \ufeffMsgBody",
		ProtocolV1Net, FramingLength, BOMModeAlways,
		SeverityWarning, FacilityAuth, "HostName", "ProcName", 12345, "MsgID", "MsgBody", "SD"},
	{"<36>Oct 11 07:25:00 HostName ProcName[12345]: MsgID MsgBody",
		ProtocolV0Net, FramingNone, BOMModeNever,
		SeverityWarning, FacilityAuth, "HostName", "ProcName", 12345, "MsgID", "MsgBody", "SD"},
	{"<36>Oct 11 07:25:00 ProcName[12345]: MsgID MsgBody",
		ProtocolV0Local, FramingNone, BOMModeNever,
		SeverityWarning, FacilityAuth, "HostName", "ProcName", 12345, "MsgID", "MsgBody", "SD"},
}

func TestProtocol(t *testing.T) {
	var fmt formatter

	refTime := time.Date(2021, 10, 11, 7, 25, 0, 0, time.UTC)
	fmt.init()

	for _, test := range encodingTests {
		var b bytes.Buffer

		//test.Expected

		err := fmt.formatTo(&b, test.Protocol, test.Framing, test.BOMMode, makePri(test.Severity, test.Facility), refTime, test.HostName, test.ProcName,
			test.PID, test.MessageID, test.MessageBody, test.StructuredData)
		if err != nil {
			t.Errorf("error: %v", err)
		}

		got := b.String()
		if got != test.Expected {
			t.Errorf("expected %q, got %q", test.Expected, got)
		}
	}
}
