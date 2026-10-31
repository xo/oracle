package ttc

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestParseXMLImage reads the images that the servers send, as
// sijms/go-ora#771 and its pull request captured them on Autonomous
// Database, and as 26ai and 11g send them.
func TestParseXMLImage(t *testing.T) {
	text := hex.EncodeToString([]byte("<a>1</a>"))
	tests := []struct {
		name    string
		image   string
		payload string
		lob     bool
		wantErr bool
	}{
		{"26ai, flags 0x14", "85 01 11 01 00000014" + text, text, false, false},
		{"ADB, flags 0x8014", "85 01 10 01 00008014" + text, text, false, false},
		{"ADB, long length", "85 01 fe 0000139b 01 00008014" + text, text, false, false},
		{"skip the next 4", "85 01 14 01 00100004 deadbeef" + text, text, false, false},
		{"with a prefix segment", "81 01 14 02 aabb 01 00000014" + text, text, false, false},
		{"11g, a CLOB locator", "85 01 5e 01 00020011 0054 0001 020c", "0054 0001 020c", true, false},
		{"unknown flags", "85 01 10 01 00000100" + text, "", false, true},
		{"degenerate", "95 01 10 01 00000014" + text, "", false, true},
		{"too short", "85 01", "", false, true},
		{"truncated length", "85 01 fe 0000", "", false, true},
		{"truncated flags", "85 01 10 01 0000", "", false, true},
	}
	for _, test := range tests {
		payload, lob, err := parseXMLImage(unhex(t, test.image))
		if (err != nil) != test.wantErr {
			t.Errorf("%s: got error %v, want an error %t", test.name, err, test.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if !bytes.Equal(payload, unhex(t, test.payload)) || lob != test.lob {
			t.Errorf("%s: got % x and a LOB %t, want %s and %t", test.name, payload, lob, test.payload, test.lob)
		}
	}
}
