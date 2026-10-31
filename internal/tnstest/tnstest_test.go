package tnstest

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"path/filepath"
	"slices"
	"testing"
)

// packet returns a packet of type typ with body, and a length of two bytes,
// or of four when large is true.
func packet(typ byte, large bool, body ...byte) []byte {
	b := make([]byte, 8, 8+len(body))
	if large {
		binary.BigEndian.PutUint32(b, uint32(8+len(body)))
	} else {
		binary.BigEndian.PutUint16(b, uint16(8+len(body)))
	}
	b[4] = typ
	return append(b, body...)
}

// accept returns an accept packet of version.
func accept(version uint16) []byte {
	return packet(typeAccept, false, byte(version>>8), byte(version))
}

func TestFileRoundTrip(t *testing.T) {
	ex := &Exchange{
		Notes: []string{"scenario: ping", "result: a", "result: b"},
		Packets: []Packet{
			{Client, packet(1, false, 'x')},
			{Server, accept(315)},
			{Client, packet(6, true, 1, 2, 3)},
		},
	}
	name := filepath.Join(t.TempDir(), "ping.tns")
	if err := ex.WriteFile(name); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Notes, ex.Notes) {
		t.Errorf("notes: got %q, want %q", got.Notes, ex.Notes)
	}
	if !slices.EqualFunc(got.Packets, ex.Packets, func(a, b Packet) bool {
		return a.Dir == b.Dir && slices.Equal(a.Data, b.Data)
	}) {
		t.Errorf("packets differ: got %v, want %v", got.Packets, ex.Packets)
	}
	if v := got.Note("scenario"); v != "ping" {
		t.Errorf("Note(scenario) = %q, want ping", v)
	}
	if v := got.NoteValues("result"); !slices.Equal(v, []string{"a", "b"}) {
		t.Errorf("NoteValues(result) = %q", v)
	}
}

func TestRecordConnSplits(t *testing.T) {
	connect, acc, data := packet(1, false, 'x'), accept(315), packet(6, true, 9, 9)
	// the chunks cut the packets at arbitrary places, as reads do
	rc := &recordConn{chunks: []chunk{
		{Client, connect[:3]},
		{Client, connect[3:]},
		{Server, acc},
		{Client, slices.Concat(data, data[:5])},
		{Client, data[5:]},
	}}
	ex, err := rc.exchange()
	if err != nil {
		t.Fatal(err)
	}
	want := []Packet{{Client, connect}, {Server, acc}, {Client, data}, {Client, data}}
	if !slices.EqualFunc(ex.Packets, want, func(a, b Packet) bool {
		return a.Dir == b.Dir && slices.Equal(a.Data, b.Data)
	}) {
		t.Errorf("got %v, want %v", ex.Packets, want)
	}
}

func TestRecordConnKeepsShortLengthBefore315(t *testing.T) {
	data := packet(6, false, 1)
	rc := &recordConn{chunks: []chunk{{Server, accept(314)}, {Client, data}}}
	ex, err := rc.exchange()
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Packets) != 2 || !slices.Equal(ex.Packets[1].Data, data) {
		t.Errorf("got %v", ex.Packets)
	}
}

func TestRecordConnRejectsTrailingBytes(t *testing.T) {
	rc := &recordConn{chunks: []chunk{{Client, packet(1, false)[:5]}}}
	if _, err := rc.exchange(); !errors.Is(err, ErrShortPacket) {
		t.Errorf("got %v, want ErrShortPacket", err)
	}
}

// client plays the client side of ex on conn, and returns the first error.
func client(conn net.Conn, ex *Exchange) error {
	var f framing
	for _, p := range ex.Packets {
		if p.Dir == Client {
			if _, err := conn.Write(p.Data); err != nil {
				return err
			}
			f.saw(p.Data)
			continue
		}
		data, err := f.read(conn)
		if err != nil {
			return err
		}
		if !slices.Equal(data, p.Data) {
			return ErrMismatch
		}
	}
	return conn.Close()
}

func TestPlay(t *testing.T) {
	ex := &Exchange{Packets: []Packet{
		{Client, packet(1, false, 'x')},
		{Server, accept(315)},
		{Client, packet(6, true, 1)},
		{Server, packet(6, true, 2, 3)},
	}}
	c, s := net.Pipe()
	errc := make(chan error, 1)
	go func() { errc <- client(c, ex) }()
	if err := Play(s, ex); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestPlayRejectsAnotherType(t *testing.T) {
	ex := &Exchange{Packets: []Packet{{Client, packet(1, false)}}}
	c, s := net.Pipe()
	go func() {
		_, _ = c.Write(packet(6, false))
		_, _ = io.Copy(io.Discard, c)
	}()
	defer c.Close()
	if err := Play(s, ex); !errors.Is(err, ErrMismatch) {
		t.Errorf("got %v, want ErrMismatch", err)
	}
}

func TestPlayRejectsAnExtraPacket(t *testing.T) {
	ex := &Exchange{Packets: []Packet{{Client, packet(1, false)}}}
	c, s := net.Pipe()
	go func() {
		_, _ = c.Write(packet(1, false))
		_, _ = c.Write(packet(6, false))
		_, _ = io.Copy(io.Discard, c)
	}()
	defer c.Close()
	if err := Play(s, ex); !errors.Is(err, ErrMismatch) {
		t.Errorf("got %v, want ErrMismatch", err)
	}
}

func TestPlayerRunsOutOfExchanges(t *testing.T) {
	p := NewPlayer()
	if _, err := p.DialContext(t.Context(), "tcp", "x"); err == nil {
		t.Error("dialing with no exchange left: no error")
	}
}
