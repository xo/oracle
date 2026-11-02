package tns

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/xo/oracle/internal/config"
)

// TestConnectDisconnectRace verifies that Session.Connect and
// Session.Disconnect do not race on session.conn when the context is
// canceled during connection establishment. Run with -race.
// See issue #736.
func TestConnectDisconnectRace(t *testing.T) {
	connOption := &config.ConnectionConfig{
		Servers: []config.ServerAddr{
			{Addr: "192.0.2.1", Port: 1521}, // TEST-NET-1: TCP handshake never completes
		},
		ServiceName:           "svc",
		SessionDataUnitSize:   0xFFFF,
		TransportDataUnitSize: 0xFFFF,
	}

	for range 5 {
		session := NewSession(connOption, slog.New(slog.DiscardHandler))
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

		var wg sync.WaitGroup
		wg.Add(2)

		// goroutine 1: Connect (writes session.conn)
		go func() {
			defer wg.Done()
			_ = session.Connect(ctx)
		}()

		// goroutine 2: wait for ctx deadline then Disconnect (reads session.conn)
		go func() {
			defer wg.Done()
			<-ctx.Done()
			session.Disconnect()
		}()

		wg.Wait()
		cancel()
	}
}

// answerConn is a net.Conn whose Write stands for a server that answers a
// break at once: the reader of the session reads the answer, and clears the
// pending break, before Write returns.
type answerConn struct {
	net.Conn
	session *Session
}

func (c answerConn) Write(b []byte) (int, error) {
	c.session.breakPending.Store(false)
	return len(b), nil
}

func (answerConn) SetWriteDeadline(time.Time) error { return nil }

// TestBreakAnsweredAtOnce checks that a break whose answer comes before the
// write of the break returns is not left pending. The session marked the
// break pending after the write, so the next call waited 10 seconds for an
// answer that had come already, and the connection went bad.
func TestBreakAnsweredAtOnce(t *testing.T) {
	session := NewSessionWithInputBufferForDebug(nil)
	session.conn = answerConn{session: session}
	if err := session.BreakConnection(); err != nil {
		t.Fatal(err)
	}
	if session.breakPending.Load() {
		t.Error("the break is pending after its answer")
	}
}
