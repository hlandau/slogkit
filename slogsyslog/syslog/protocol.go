package syslog

import (
	"fmt"
	"io"
	"time"
)

// The standard UDP port for SYSLOG.
const DefaultPort = 514

// Specifies a SYSLOG protocol variant.
type Protocol int

const (
	// Select a protocol automatically.
	ProtocolAuto Protocol = iota
	// Use the SYSLOGv0-LOCAL protocol with old timestamps and no hostname field.
	ProtocolV0Local
	// Use the SYSLOGv0-NET protocol with old timestamps and a hostname field.
	ProtocolV0Net
	// Use the SYSLOGv1-NET protocol.
	ProtocolV1Net
)

func (p Protocol) isLocal() bool {
	return p == ProtocolV0Local
}

func (p Protocol) isV1() bool {
	return p == ProtocolV1Net
}

func (p Protocol) resolve(isUnix bool) Protocol {
	if p != ProtocolAuto {
		return p
	}

	if isUnix {
		return ProtocolV0Local
	}

	return ProtocolV1Net
}

// Specifies a SYSLOG protocol framing variant.
type Framing int

const (
	// Select a framing automatically using reasonable defaults.
	FramingAuto Framing = iota

	// Use explicit length framing.
	FramingLength

	// Use a NUL byte as a delimiter.
	FramingDelimiterNUL

	// Use a LF byte as a delimiter.
	FramingDelimiterLF

	// No framing. For UDP/UNIX use only. You do not normally need to select this
	// yourself; expert use only.
	FramingNone
)

func (f Framing) resolve(needFraming bool) Framing {
	if !needFraming {
		return FramingNone
	}

	if f == FramingAuto {
		return FramingDelimiterNUL
	}
	return f
}

// Specifies whether to use a UTF-8 BOM in message body fields.
type BOMMode int

const (
	// Automatically determine whether to use a UTF-8 BOM based on reasonable defaults.
	BOMModeAuto BOMMode = iota

	// Always include a UTF-8 BOM.
	BOMModeAlways

	// Never include a UTF-8 BOM.
	BOMModeNever
)

func (b BOMMode) resolve(p Protocol) BOMMode {
	if b != BOMModeAuto {
		return b
	}

	if p.isV1() {
		return BOMModeAlways
	} else {
		return BOMModeNever
	}
}

type formatter struct {
	writeBuf []byte
}

func (fmtr *formatter) init() {
	fmtr.writeBuf = make([]byte, 16)
	fmtr.writeBuf[15] = ' '
}

// Generates a SYSLOG protocol message using the given protocol.
func (fmtr *formatter) formatTo(w io.Writer, p Protocol, f Framing, b BOMMode, pri int, timestamp time.Time, hostName, procName string, procID int, msgID, msgBody, structuredData string) error {
	buf := fmtr.writeBuf[0:16]

	// Empty Fields
	if hostName == "" {
		hostName = "-"
	}
	if procName == "" {
		procName = "-"
	}
	if structuredData == "" {
		structuredData = "-"
	}

	// Framing
	endChar := ""
	switch f {
	case FramingDelimiterNUL:
		endChar = "\x00"
	case FramingDelimiterLF:
		endChar = "\n"
	default:
		endChar = ""
	}

	// BOM
	bomPfx := ""
	if b == BOMModeAlways {
		bomPfx = "\xEF\xBB\xBF"
	}

	// Protocol Version
	switch p {
	case ProtocolV0Local:
		sep := ""
		if msgID != "" {
			sep = " "
		}
		// Message ID is folded into message body for v0, so BOM comes before it.
		buf = fmt.Appendf(buf, "<%d>%s %s[%d]: %s%s%s%s%s", pri, timestamp.Format(time.Stamp), procName, procID, bomPfx, msgID, sep, msgBody, endChar)
	case ProtocolV0Net:
		sep := ""
		if msgID != "" {
			sep = " "
		}
		// Message ID is folded into message body for v0, so BOM comes before it.
		buf = fmt.Appendf(buf, "<%d>%s %s %s[%d]: %s%s%s%s%s", pri, timestamp.Format(time.Stamp), hostName, procName, procID, bomPfx, msgID, sep, msgBody, endChar)
	case ProtocolV1Net:
		if msgID == "" {
			msgID = "-"
		}
		buf = fmt.Appendf(buf, "<%d>1 %s %s %s %d %s %s %s%s%s", pri, timestamp.Format(time.RFC3339Nano), hostName, procName, procID, msgID, structuredData, bomPfx, msgBody, endChar)
	default:
		panic("unknown syslog protocol")
	}

	// Optional Length Framing
	if f != FramingLength {
		_, err := w.Write(buf[16:])
		return err
	}

	// Encode decimal integer.
	lrem := len(buf[16:])
	i := 15
	for lrem > 0 {
		i--
		buf[i] = '0' + byte(lrem%10)
		lrem /= 10
	}

	_, err := w.Write(buf[i:])
	fmtr.writeBuf = buf
	return err
}
