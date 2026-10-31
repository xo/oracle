package tnstest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// playTimeout bounds one exchange, so that a driver that waits for a packet
// that the exchange does not hold fails the test and does not hang it.
const playTimeout = time.Minute

// Player plays exchanges back to the driver, one for each connection that
// the driver dials, in order. It satisfies the dialer interface of the
// driver, and it opens no socket: each connection is one end of a net.Pipe.
type Player struct {
	mu        sync.Mutex
	exchanges []*Exchange
	wg        sync.WaitGroup
	errs      []error
}

// NewPlayer returns a Player for exchanges.
func NewPlayer(exchanges ...*Exchange) *Player {
	return &Player{exchanges: exchanges}
}

// DialContext returns the client end of a pipe, and plays the next exchange
// on the other end. It ignores the address.
func (p *Player) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.exchanges) == 0 {
		return nil, fmt.Errorf("dialing: %w", ErrNoExchange)
	}
	ex := p.exchanges[0]
	p.exchanges = p.exchanges[1:]
	client, server := net.Pipe()
	deadline := time.Now().Add(playTimeout)
	if err := errors.Join(client.SetDeadline(deadline), server.SetDeadline(deadline)); err != nil {
		return nil, fmt.Errorf("dialing: %w", err)
	}
	p.wg.Go(func() {
		defer server.Close()
		if err := Play(server, ex); err != nil {
			p.mu.Lock()
			p.errs = append(p.errs, err)
			p.mu.Unlock()
		}
	})
	return client, nil
}

// Wait waits until every exchange that started ends, and returns their
// errors, joined.
func (p *Player) Wait() error {
	p.wg.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	return errors.Join(p.errs...)
}

// clientPacket is one packet that the client sent, or the error that ended
// the reading.
type clientPacket struct {
	data []byte
	err  error
}

// Play plays ex on conn, as the server. It reads each packet that the client
// sends and compares its type with the recorded one, and it writes each
// packet that the server sent. The content of a client packet differs from
// run to run, because it holds random keys and the time, so only its type
// and its place in the order are compared.
//
// A goroutine reads the packets of the client as they arrive, because a pipe
// holds no buffer. Over TCP the kernel holds what the client writes while the
// server writes too, as when the server sends a marker and the client answers
// it with one, and without the goroutine both writes wait for each other.
//
// When the exchange ends, Play reads until the client closes the
// connection, and a packet that arrives then is an error.
func Play(conn net.Conn, ex *Exchange) error {
	var large atomic.Bool
	in := make(chan clientPacket)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			data, err := readPacket(conn, large.Load)
			select {
			case in <- clientPacket{data, err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for i, p := range ex.Packets {
		switch p.Dir {
		case Client:
			cp := <-in
			if cp.err != nil {
				return fmt.Errorf("reading packet %d from the client: %w", i, cp.err)
			}
			if cp.data[4] != p.Type() {
				return fmt.Errorf("reading packet %d from the client, of type %d where the exchange has type %d: %w", i, cp.data[4], p.Type(), ErrMismatch)
			}
		case Server:
			// the client can answer as soon as it reads the packet, so the
			// framing changes before the write
			if isLargeAccept(p.Data) {
				large.Store(true)
			}
			if _, err := conn.Write(p.Data); err != nil {
				return fmt.Errorf("writing packet %d to the client: %w", i, err)
			}
		}
	}
	cp := <-in
	switch {
	case errors.Is(cp.err, io.EOF), errors.Is(cp.err, io.ErrClosedPipe):
		return nil
	case cp.err != nil:
		return fmt.Errorf("reading after the end of the exchange: %w", cp.err)
	}
	return fmt.Errorf("reading a packet of type %d after the end of the exchange: %w", cp.data[4], ErrMismatch)
}
