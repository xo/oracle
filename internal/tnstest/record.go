package tnstest

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// Recorder dials a real server and records every byte that passes on each
// connection that it dials. It satisfies the dialer interface of the driver.
type Recorder struct {
	// Addr is the address of the server. When it is set, the Recorder dials
	// it in place of the address that the driver asks for, so that the
	// driver can name a fixed address that a recording holds.
	Addr string

	mu    sync.Mutex
	conns []*recordConn
}

// DialContext dials address and returns a connection that records.
func (rec *Recorder) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if rec.Addr != "" {
		address = rec.Addr
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	rc := &recordConn{Conn: conn}
	rec.mu.Lock()
	rec.conns = append(rec.conns, rc)
	rec.mu.Unlock()
	return rc, nil
}

// Exchanges returns the packets of each connection, in the order dialed.
func (rec *Recorder) Exchanges() ([]*Exchange, error) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	exs := make([]*Exchange, 0, len(rec.conns))
	for i, rc := range rec.conns {
		ex, err := rc.exchange()
		if err != nil {
			return nil, fmt.Errorf("splitting connection %d into packets: %w", i, err)
		}
		exs = append(exs, ex)
	}
	return exs, nil
}

// chunk is the bytes of one call to Read or Write.
type chunk struct {
	dir  Dir
	data []byte
}

// recordConn keeps each chunk that passes, in the order it passed.
type recordConn struct {
	net.Conn

	mu     sync.Mutex
	chunks []chunk
}

func (rc *recordConn) Read(b []byte) (int, error) {
	n, err := rc.Conn.Read(b)
	if n > 0 {
		rc.mu.Lock()
		rc.chunks = append(rc.chunks, chunk{Server, append([]byte(nil), b[:n]...)})
		rc.mu.Unlock()
	}
	return n, err
}

func (rc *recordConn) Write(b []byte) (int, error) {
	n, err := rc.Conn.Write(b)
	if n > 0 {
		rc.mu.Lock()
		rc.chunks = append(rc.chunks, chunk{Client, append([]byte(nil), b[:n]...)})
		rc.mu.Unlock()
	}
	return n, err
}

// exchange splits the chunks into packets. A side switches to the long
// length when the accept packet passes, and the client only writes after it
// read the accept, so the order of the chunks is the order of the framing.
func (rc *recordConn) exchange() (*Exchange, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var (
		f   framing
		buf = map[Dir][]byte{}
		ex  = new(Exchange)
	)
	for _, c := range rc.chunks {
		buf[c.dir] = append(buf[c.dir], c.data...)
		for {
			data, rest, err := f.next(buf[c.dir])
			if err != nil {
				return nil, err
			}
			if data == nil {
				break
			}
			buf[c.dir] = rest
			ex.Packets = append(ex.Packets, Packet{Dir: c.dir, Data: append([]byte(nil), data...)})
			f.saw(data)
		}
	}
	for dir, b := range buf {
		if len(b) != 0 {
			return nil, fmt.Errorf("%d bytes from %c after the last packet: %w", len(b), dir, ErrShortPacket)
		}
	}
	return ex, nil
}
