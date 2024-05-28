// Package syslog provides a library for sending messages to SYSLOG servers.
//
// # Protocol Variations
//
// There are several versions of the SYSLOG protocol, not all of which are well
// documented. For the purposes of this discussion, this package has
// arbitrarily named them as follows:
//
//   - SYSLOGv0-LOCAL: This uses an RFC 822-ish timestamp but without a year
//     field, but unlike SYSLOGv0-NET, has no hostname field. It is used when
//     communicating with a local SYSLOG daemon via a UNIX domain socket.
//
//   - SYSLOGv0-NET (RFC 3164): This uses an RFC 822-style timestamp without a
//     year field. This probably remains the most popular variant.
//
//   - SYSLOGv1-NET (RFC 5424): This uses an RFC 3339-style timestamp and features a message ID field
//     as well as optional support for structured data.
//
// Broken down by feature:
//
//	Feature          v0L  v0N  v1N
//	---------------  ---  ---  ---
//	Timestamp        Old  Old  New
//	Message Body     X    X    X
//	Process Name     X    X    X
//	Process ID       X    X    X
//	Hostname              X    X
//	Message ID                 X
//	Structured Data            X
//
// Unless a protocol is explicitly specified by the user, this package will
// automatically select SYSLOGv0-LOCAL for connections which appear to be using
// UNIX domain sockets and SYSLOGv1-NET otherwise.
//
// # Message Framing
//
// A further complication arises with the question of message framing.
// Traditionally, SYSLOG specifies no particular message framing. For
// transmission of SYSLOG protocol messages over a UNIX domain or UDP socket,
// this poses no issue; UNIX domain sockets can be used in SOCK_DGRAM mode and
// one UNIX domain or UDP datagram can be sent per SYSLOG message. For transport
// of SYSLOG protocol messages over TCP or TLS however, a delimiter character such
// as a newline has traditionally been used. This is troublesome because it means
// the message body of a SYSLOG protocol message cannot contain that delimiter.
//
// RFC 6587 provides several options here:
//
//   - Length Framing: This is an explicit length framing which is completely
//     unambiguous. Not all SYSLOG sinks may support it.
//
//   - Delimiters: This is a delimiter value such as a newline or NUL byte.
//     An implication of this is that SYSLOG protocol message fields cannot
//     contain this delimiter character. However, it is likely to be more
//     compatible.
//
// Unless a framing is explicitly specified by the user, this package will
// automatically select delimiter-based framing using a NUL byte. This allows
// arbitrary UTF-8 text, including newlines, to be incorporated in messages.
//
// # UTF-8 and BOM Insertion
//
// SYSLOGv1 permits usage of UTF-8, but requires that a UTF-8 BOM be inserted
// before any message body which uses UTF-8. Since all Go strings are UTF-8,
// UTF-8 is used regardless of whether a BOM is inserted or not. This package
// allows the user to choose whether to insert a BOM in messages. By default,
// this package inserts a BOM only if using SYSLOGv1.
//
// # Reliability and Reconnection
//
// SYSLOG is not a reliable protocol.
//
// This package will attempt to reconnect to a SYSLOG server if a connection is
// lost. Initially, one reconnection attempt is made per Logger.Write() call;
// if that fails, the Write() call will return with an error.
//
// In order to prevent pathological performance for applications in the event of
// a SYSLOG server failure, reconnection attempts are rate limited. This means
// that after a reconnection attempt fails and Logger.Write() returns an error,
// any subsequent calls to Logger.Write() will also automatically fail for a
// certain period of time. This backoff period is configurable.
//
// This package does not perform any buffering of syslog messages (for example,
// so that messages can be held when a server goes down until it comes back
// up). The application may undertake to buffer syslog messages if it wishes. A
// better solution may be to investigate running a local syslog daemon which
// can take responsibility for further transport of syslog messages over the
// network.
//
// # Missing Features
//
// This package does not currently support generation of SYSLOGv1 structured
// data, although there is minimal support for incorporating such data in a
// Message structure if you are capable of serializing SYSLOGv1 structured data
// yourself.
//
// TLS support is not included out of the box to keep package dependencies
// down for applications which do not need it. You can plug this in yourself
// if needed by providing a custom DialFunc.
//
// # OS Support
//
// Unlike the Go log/syslog package, this package supports network-based SYSLOG
// usage on any platform. The usage of UNIX domain sockets is of course only
// supported on UNIX platforms.
package syslog

import (
	gnet "github.com/hlandau/goutils/net"

	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Represents a SYSLOG protocol message.
type Message struct {
	// The message timestamp. If this is the zero value, it is set to the current
	// time automatically.
	Time time.Time

	// The SYSLOG severity the message is categorised under.
	Severity Severity

	// The SYSLOG facility the message is categorised under.
	Facility Facility

	// The message ID. This may be empty.
	//
	// For protocol versions without a separate message ID field, this is merged
	// with the message body if it is non-empty by prepending this field followed
	// by a space.
	ID string

	// The message body.
	Body string

	// Encoded SYSLOGv1 structured data. This may be empty.
	StructuredData string
}

// Syslog writer configuration.
type Config struct {
	// The syslog protocol to use. By default this is ProtocolAuto and a
	// reasonable default will be chosen depending on the circumstances.
	Protocol Protocol

	// The syslog framing to use. This is only relevant for network channels
	// with byte-oriented semantics (e.g., TCP, TLS).
	Framing Framing

	// Determines whether a UTF-8 BOM should be placed before every message body.
	// By default (BOMModeAuto) this is determined automatically; a BOM is always
	// used for SYSLOGv1 but never for SYSLOGv0.
	BOMMode BOMMode

	// Determines how long we will wait to reconnect after a connection failure.
	ConnectBackoff gnet.Backoff

	// If this is non-nil, this is called when a new connection is required. This
	// may be when connecting for the first time, or when reconnecting.
	// Otherwise, Dialer is used. The context passed is the context passed to
	// Logger.Write() or New().
	DialFunc func(ctx context.Context, net, addr string) (io.WriteCloser, error)

	// Dial-style network string.
	//
	// Valid values are "udp", "tcp", "unix" and "unixgram".
	//
	// If both Network and Address are left empty, this defaults to "unix".
	// Otherwise, it defaults to "unixgram" or "udp" based on whether the content
	// of Address appears to be a path or not.
	Network string

	// Dial-style address string.
	//
	// For "unix" or "unixgram", this should be a path to a UNIX domain socket.
	// If it is left empty, a number of standard UNIX paths for a system SYSLOG
	// daemon are tried in sequence.
	//
	// For any other network, this should be a "hostname:port" combination.
	// If the port is omitted, the SYSLOG default port is tried.
	Address string

	// The hostname to use when logging messages. If empty, this is set
	// to the machine's hostname automatically. To avoid specifying a hostname,
	// specify "-". Must not contain whitespace.
	HostName string

	// The process name to use when logging messages. If empty, this is set to
	// the detected process name automatically. To avoid specifying a process
	// name, specify "-". Must not contain whitespace.
	ProcName string
}

// A syslog log writer.
type Logger struct {
	cfg                Config
	w                  io.WriteCloser
	connTargets        []connTarget
	closed             bool
	reconnectStartTime time.Time
	autoconfigDone     bool
	fmtr               formatter
	mutex              sync.Mutex
}

// Creates a new SYSLOG protocol writer, which connects and reconnects
// automatically as needed to a SYSLOG server.
func New(cfg Config) (*Logger, error) {
	l := &Logger{
		cfg: cfg,
	}

	var err error
	l.connTargets, err = determineConnTargets(l.cfg.Network, l.cfg.Address)
	if err != nil {
		return nil, err
	}

	l.fmtr.init()
	return l, nil
}

type connTarget struct {
	Network, Address string
}

func determineConnTargets(network, address string) ([]connTarget, error) {
	connTargets, err := determineOSSpecificConnTargets(network, address)
	if connTargets != nil || err != nil {
		return connTargets, err
	}

	if address == "" {
		return nil, errors.New("no syslog address specified")
	}

	if network == "" {
		network = "udp"
	}

	hasPort, err := gnet.HasPort(address)
	if err != nil {
		return nil, err
	}

	if !hasPort {
		address += fmt.Sprintf(":%d", DefaultPort)
	}

	return []connTarget{{network, address}}, nil
}

func (l *Logger) getNewConnUsingTarget(ctx context.Context, network, address string) (io.WriteCloser, error) {
	if l.cfg.DialFunc != nil {
		return l.cfg.DialFunc(ctx, network, address)
	}

	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func (l *Logger) getNewConn(ctx context.Context) (io.WriteCloser, error) {
	var firstErr error
	for _, connTarget := range l.connTargets {
		w, err := l.getNewConnUsingTarget(ctx, connTarget.Network, connTarget.Address)
		if err == nil {
			return w, nil
		}

		if firstErr == nil {
			firstErr = err
		}
	}

	return nil, firstErr
}

type hasLocalAddr interface {
	LocalAddr() net.Addr
}

func (l *Logger) getNetwork(w io.WriteCloser) string {
	if laW, ok := w.(hasLocalAddr); ok {
		la := laW.LocalAddr()
		if la != nil {
			return la.Network()
		}
	}
	return l.connTargets[0].Network
}

func isUnix(network string) bool {
	return strings.HasPrefix(network, "unix")
}

func needsFraming(network string) bool {
	switch network {
	case "unix", "unixgram", "udp":
		return false
	default:
		return true
	}
}

func (l *Logger) autoconfig() error {
	if l.autoconfigDone {
		return nil
	}

	actualNetwork := l.getNetwork(l.w)
	l.cfg.Protocol = l.cfg.Protocol.resolve(isUnix(actualNetwork))
	l.cfg.Framing = l.cfg.Framing.resolve(needsFraming(actualNetwork))
	l.cfg.BOMMode = l.cfg.BOMMode.resolve(l.cfg.Protocol)

	if l.cfg.HostName == "" {
		l.cfg.HostName, _ = os.Hostname()
	}

	if l.cfg.ProcName == "" {
	}

	return nil
}

func (l *Logger) ensureConn(ctx context.Context) error {
	if l.w != nil {
		return nil
	}

	if l.closed {
		return errClosed
	}

	if !l.reconnectStartTime.IsZero() && time.Now().Before(l.reconnectStartTime) {
		return errReconnectBackoff
	}

	l.reconnectStartTime = time.Now().Add(l.cfg.ConnectBackoff.NextDelay())

	var err error
	l.w, err = l.getNewConn(ctx)
	if err == nil {
		l.autoconfig()
	}

	return err
}

func (l *Logger) destroyConn() {
	if l.w == nil {
		return
	}

	l.w.Close()
	l.w = nil
}

// Closes the syslog writer, as well as any underlying network connection.
// Future calls to Write will fail. This function is idempotent.
func (l *Logger) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.destroyConn()
	l.closed = true
	return nil
}

func makePri(severity Severity, facility Facility) int {
	return (int(severity) & 7) + ((int(facility) << 3) & 0xf8)
}

var (
	errClosed           = errors.New("writing to a closed syslog logger")
	errReconnectBackoff = errors.New("syslog logger is waiting to reconnect")
)

// Writes a message to the underlying SYSLOG protocol connection at once. No
// buffering is performed.
//
// This will automatically attempt to reconnect to the server if the connection
// is lost (see package comment for details). The passed context strictly
// bounds the time spent performing reconnection attempts, but does not bound
// the time spent writing any messages to a healthy connection. The premise
// here is that if a SYSLOG transport with flow control (e.g. TCP) does exhibit
// backpressure, it does not really make any sense to end up logging only half
// a log message, and indeed this will cause breakage depending on the framing
// used.
//
// Calls are synchronised and thread safe.
func (l *Logger) Write(ctx context.Context, msg Message) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	err := l.ensureConn(ctx)
	if err != nil {
		return err
	}

	timestamp := msg.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	pri := makePri(msg.Severity, msg.Facility)

	for i := 0; ; i++ {
		err := l.fmtr.formatTo(l.w, l.cfg.Protocol, l.cfg.Framing, l.cfg.BOMMode, pri, timestamp, l.cfg.HostName, l.cfg.ProcName, os.Getpid(), msg.ID, msg.Body, msg.StructuredData)
		if err == nil {
			l.cfg.ConnectBackoff.Reset()
		}
		if err == nil {
			return nil
		}
		if i > 0 {
			l.destroyConn()
			return err
		}

		l.destroyConn()
		err2 := l.ensureConn(ctx)
		if err2 != nil {
			return err
		}
	}
}
