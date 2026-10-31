// Package tnstest records the TNS packets that the driver and a server
// exchange, and plays them back to the driver with no server. Only tests
// import it. See D15 and docs/DESIGN.md, under Testing.
package tnstest

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// Error is the type of the sentinel errors of this package.
type Error string

// Error satisfies the error interface.
func (err Error) Error() string {
	return string(err)
}

// The errors of this package.
const (
	// ErrMismatch is a packet from the driver that differs from the one that
	// the exchange holds.
	ErrMismatch Error = "packet differs from the recorded exchange"
	// ErrShortPacket is a packet whose length is less than its header.
	ErrShortPacket Error = "packet shorter than its header"
	// ErrFormat is a line of an exchange file that cannot be read.
	ErrFormat Error = "malformed exchange file"
	// ErrNoExchange is a dial after the Player played every exchange.
	ErrNoExchange Error = "no recorded exchange is left"
)

// Dir is the side that sent a packet.
type Dir byte

// The two sides.
const (
	Client Dir = 'C'
	Server Dir = 'S'
)

// typeAccept is the TNS packet type of the accept packet, which ends the
// handshake.
const typeAccept byte = 2

// Packet is one TNS packet, with its header.
type Packet struct {
	Dir  Dir
	Data []byte
}

// Type returns the TNS packet type.
func (p Packet) Type() byte {
	if len(p.Data) < 5 {
		return 0
	}
	return p.Data[4]
}

// Exchange is the packets of one connection, in the order they were sent,
// with the notes that describe it. A note is a line of text, such as the
// release of the server and the result that the test expects.
type Exchange struct {
	Notes   []string
	Packets []Packet
}

// Note returns the value of the first note that starts with key and a colon,
// with the space after the colon removed.
func (ex *Exchange) Note(key string) string {
	for _, n := range ex.Notes {
		if v, ok := strings.CutPrefix(n, key+":"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// NoteValues returns every value of the notes that start with key and a colon.
func (ex *Exchange) NoteValues(key string) []string {
	var vals []string
	for _, n := range ex.Notes {
		if v, ok := strings.CutPrefix(n, key+":"); ok {
			vals = append(vals, strings.TrimSpace(v))
		}
	}
	return vals
}

// WriteFile writes the exchange as text: each note as a line that starts with
// "# ", then each packet as its side, a space and its bytes in hex.
func (ex *Exchange) WriteFile(name string) error {
	var b strings.Builder
	for _, n := range ex.Notes {
		fmt.Fprintf(&b, "# %s\n", n)
	}
	for _, p := range ex.Packets {
		fmt.Fprintf(&b, "%c %x\n", p.Dir, p.Data)
	}
	return os.WriteFile(name, []byte(b.String()), 0o644)
}

// ReadFile reads an exchange that WriteFile wrote.
func ReadFile(name string) (*Exchange, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ex := new(Exchange)
	s := bufio.NewScanner(f)
	s.Buffer(nil, 64<<20)
	for n := 1; s.Scan(); n++ {
		line := s.Text()
		switch {
		case line == "":
		case strings.HasPrefix(line, "# "):
			ex.Notes = append(ex.Notes, line[2:])
		case len(line) > 2 && (line[0] == byte(Client) || line[0] == byte(Server)) && line[1] == ' ':
			data, err := hex.DecodeString(line[2:])
			if err != nil {
				return nil, fmt.Errorf("reading %s line %d: %w: %w", name, n, ErrFormat, err)
			}
			ex.Packets = append(ex.Packets, Packet{Dir: Dir(line[0]), Data: data})
		default:
			return nil, fmt.Errorf("reading %s line %d: %w", name, n, ErrFormat)
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	return ex, nil
}

// framing reads TNS packets from a stream. Before the handshake, the length
// of a packet is the first two bytes of its header. After an accept packet
// whose version is 315 or later, it is the first four bytes, on both sides.
type framing struct {
	large bool
}

// next returns the first packet in buf and the bytes after it. It returns a
// nil packet when buf does not yet hold a whole packet.
func (f *framing) next(buf []byte) ([]byte, []byte, error) {
	if len(buf) < 8 {
		return nil, buf, nil
	}
	n := int(binary.BigEndian.Uint16(buf))
	if f.large {
		n = int(binary.BigEndian.Uint32(buf))
	}
	if n < 8 {
		return nil, buf, fmt.Errorf("reading a packet of length %d: %w", n, ErrShortPacket)
	}
	if len(buf) < n {
		return nil, buf, nil
	}
	return buf[:n:n], buf[n:], nil
}

// saw updates the framing after a packet passes.
func (f *framing) saw(data []byte) {
	if isLargeAccept(data) {
		f.large = true
	}
}

// isLargeAccept reports whether data is an accept packet of version 315 or
// later, after which a packet has a length of four bytes.
func isLargeAccept(data []byte) bool {
	return len(data) >= 10 && data[4] == typeAccept && binary.BigEndian.Uint16(data[8:]) >= 315
}

// read reads one whole packet from r.
func (f *framing) read(r io.Reader) ([]byte, error) {
	data, err := readPacket(r, func() bool { return f.large })
	if err != nil {
		return nil, err
	}
	f.saw(data)
	return data, nil
}

// readPacket reads one whole packet from r. Its length is the first four
// bytes of its header when large reports true, and the first two when it
// does not. large is called after the header arrives, because a reader that
// waits for the header can start before the accept packet passes.
func readPacket(r io.Reader, large func() bool) ([]byte, error) {
	head := make([]byte, 8)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(head))
	if large() {
		n = int(binary.BigEndian.Uint32(head))
	}
	if n < 8 {
		return nil, fmt.Errorf("reading a packet of length %d: %w", n, ErrShortPacket)
	}
	data := make([]byte, n)
	copy(data, head)
	if _, err := io.ReadFull(r, data[8:]); err != nil {
		return nil, err
	}
	return data, nil
}
