package syslog

import (
	"context"
	"fmt"
	"testing"
)

// test
//
//	"",""
//	"","/dev/log"
//	"unix",""
//	"unixgram",""
//	"unix","/dev/log"
//	"unixgram","/dev/log"
//	"","foobar"
//	"","foobar.example.com"
//	"","foobar.example.com:514"
//	"","127.0.0.1"
//	"","127.0.0.1:514",
//	"","[::1]"
//	"","[::1]:514"
//	"tcp","foobar.example.com"
//	"tcp","foobar.example.com:514"
//	"udp","foobar.example.com"
//	"udp","foobar.example.com:514"

/*
Test that it
  - can write a syslog message
  - can write multiple syslog messages of varying length
  - can use length framing
  - can use NUL framing
  - can use LF framing
  - can use BOM prefixing
  - can use each supported protocol version
  - can use UNIX sockets with path autodetect
  - can use UNIX sockets with manual path
  - can use UDP socket with default port
  - can use UDP socket with different port
  - can use UDP socket with IPv6 and default port
  - can use UDP socket with IPv6 and different port
  - can use TCP socket
  - can reconnect correctly
  - does reconnection backoff correctly
*/
func TestSyslog(t *testing.T) {
	log, err := New(Config{
		// By default, we connect to the local syslog daemon via a UNIX domain socket.
		//
		// Uncomment the below to use a UDP destination.
		//Address: "127.0.0.1:514",
		//Network: "tcp",
		//Framing: FramingLength,
	})
	if err != nil {
		t.Errorf("cannot instantiate: %v", err)
		return
	}

	for i := 0; i < 2; i++ {
		err = log.Write(context.Background(), Message{
			Severity: SeverityDebug,
			Facility: FacilityDaemon,
			Body:     fmt.Sprintf("This is syslog message %d.", i),
		})
		if err != nil {
			t.Errorf("cannot write: %v", err)
			return
		}
		//time.Sleep(1 * time.Second)
	}
}
