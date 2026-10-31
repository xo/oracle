package tns

import "testing"

// TestReadInt64Size is sijms/go-ora#624: a compressed integer whose length
// byte is over 8 panicked with slice bounds out of range [-68:].
func TestReadInt64Size(t *testing.T) {
	tests := []struct {
		in      []byte
		want    int64
		wantErr bool
	}{
		{[]byte{0}, 0, false},
		{[]byte{2, 1, 2}, 0x102, false},
		{[]byte{0x82, 1, 2}, -0x102, false},
		{[]byte{76, 1, 2, 3}, 0, true},
		{[]byte{9, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 0, true},
		{[]byte{4, 1}, 0, true},
	}
	for _, test := range tests {
		s := NewMemorySession(test.in, nil, SessionProperties{})
		got, err := s.ReadInt64(4, true, true)
		if (err != nil) != test.wantErr || got != test.want {
			t.Errorf("ReadInt64 of % x: got %d, %v, want %d and an error %t", test.in, got, err, test.want, test.wantErr)
		}
	}
}

func TestReadStringLength(t *testing.T) {
	s := NewMemorySession([]byte{3, 'a', 'b', 'c'}, nil, SessionProperties{})
	if got, err := s.ReadString(2); err != nil || got != "ab" {
		t.Errorf("got %q, %v, want ab", got, err)
	}
	s = NewMemorySession([]byte{3, 'a', 'b', 'c'}, nil, SessionProperties{})
	if _, err := s.ReadString(5); err == nil {
		t.Error("a length longer than the string gives no error")
	}
}
