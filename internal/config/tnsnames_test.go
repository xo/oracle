package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLookupAlias(t *testing.T) {
	const dir = "testdata/tnsnames"
	tests := []struct {
		alias, service, sid, host string
		port                      int
		protocol                  string
	}{
		{alias: "orcl", service: "orclpdb1", host: "db.example.com", port: 1521, protocol: "TCP"},
		{alias: "SALES", sid: "SALES", host: "s1", port: 1522, protocol: "TCP"},
		{alias: "sales.example.com", sid: "SALES", host: "s1", port: 1522, protocol: "TCP"},
		{alias: "secure", service: "secure", host: "sec.example.com", port: 2484, protocol: "TCPS"},
	}
	for _, test := range tests {
		desc, err := LookupAlias(dir, test.alias)
		if err != nil {
			t.Fatalf("%s: %v", test.alias, err)
		}
		cfg := New()
		if err := cfg.UpdateDatabaseInfo(desc); err != nil {
			t.Fatalf("%s: %s: %v", test.alias, desc, err)
		}
		if cfg.ServiceName != test.service || cfg.SID != test.sid {
			t.Errorf("%s: got the service %q and the SID %q", test.alias, cfg.ServiceName, cfg.SID)
		}
		if len(cfg.Servers) == 0 || cfg.Servers[0].Addr != test.host || cfg.Servers[0].Port != test.port || cfg.Servers[0].Protocol != test.protocol {
			t.Errorf("%s: got the servers %+v", test.alias, cfg.Servers)
		}
	}
	if desc, _ := LookupAlias(dir, "sales"); desc == "" {
		t.Error("sales has no descriptor")
	} else if cfg := New(); cfg.UpdateDatabaseInfo(desc) != nil || len(cfg.Servers) != 2 {
		t.Errorf("sales: got the servers %+v, want 2", cfg.Servers)
	}
	if _, err := LookupAlias(dir, "nothere"); !errors.Is(err, ErrNoAlias) {
		t.Errorf("an unknown alias: got %v", err)
	}
	if _, err := LookupAlias("testdata/tnsbad", "broken"); err == nil {
		t.Error("an unclosed descriptor gives no error")
	}
	if _, err := LookupAlias("testdata/none", "x"); err == nil {
		t.Error("a missing file gives no error")
	}
}

// writeFiles writes each file of files, keyed by its name, in dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLookupAliasCases holds the cases of tnsnames.ora that python-oracledb
// tests, in tests/misc/test_1500_tnsnames.py, written again here. Each names
// the number of its test there.
func TestLookupAliasCases(t *testing.T) {
	const desc = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h)(PORT=7200))(CONNECT_DATA=(SERVICE_NAME=s)))"
	other := t.TempDir()
	writeFiles(t, other, map[string]string{"inc.ora": "nsn_b = host_b:7208/service_b"})
	tests := []struct {
		name  string
		files map[string]string
		alias string
		// host and service are what the alias names, and err is true when
		// the lookup fails
		host, port, service string
		err                 bool
	}{
		{name: "1500 a simple entry", files: map[string]string{"tnsnames.ora": "nsn = \n  (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h)(PORT=7200))\n  (CONNECT_DATA=(SERVICE_NAME=s)))"},
			alias: "nsn", host: "h", port: "7200", service: "s"},
		{name: "1501 a missing entry", files: map[string]string{"tnsnames.ora": "# no entries"}, alias: "nsn", err: true},
		{name: "1502 no tnsnames.ora", files: map[string]string{}, alias: "nsn", err: true},
		{name: "1503 text that is no entry", files: map[string]string{"tnsnames.ora": "some garbage data which is not a valid entry\nnsn = " + desc + "\n"},
			alias: "nsn", host: "h", port: "7200", service: "s"},
		{name: "1504 two aliases on a line", files: map[string]string{"tnsnames.ora": "nsn_a,nsn_b = " + desc},
			alias: "NSN_B", host: "h", port: "7200", service: "s"},
		{name: "1505 an easy connect string", files: map[string]string{"tnsnames.ora": "nsn = tcp://host_7205:7205/service_7205"},
			alias: "nsn", host: "host_7205", port: "7205", service: "service_7205"},
		{name: "1506 a descriptor with a /", files: map[string]string{"tnsnames.ora": "nsn = (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h)(PORT=7206))(CONNECT_DATA=(SERVICE_NAME=s))(SECURITY=(MY_WALLET_DIRECTORY=/some/dir/7206)))"},
			alias: "nsn", host: "h", port: "7206", service: "s"},
		{name: "1507 IFILE in the same folder", files: map[string]string{"tnsnames.ora": "nsn_a = host_a:7207/service_a\nifile = inc.ora", "inc.ora": "nsn_b = host_b:7207/service_b"},
			alias: "nsn_b", host: "host_b", port: "7207", service: "service_b"},
		{name: "1508 IFILE in another folder", files: map[string]string{"tnsnames.ora": "nsn_a = host_a:7208/service_a\nifile = " + filepath.Join(other, "inc.ora")},
			alias: "nsn_b", host: "host_b", port: "7208", service: "service_b"},
		{name: "1509 a file that includes itself", files: map[string]string{"tnsnames.ora": "nsn = some_host/some_service\nIFILE = tnsnames.ora"}, alias: "nsn", err: true},
		{name: "1510 a cycle through an included file", files: map[string]string{"tnsnames.ora": "nsn = some_host/some_service\nIFILE = inc.ora", "inc.ora": "IFILE = tnsnames.ora"}, alias: "nsn", err: true},
		{name: "1511 a cycle through two included files", files: map[string]string{"tnsnames.ora": "nsn = some_host/some_service\nIFILE = a.ora", "a.ora": "IFILE = b.ora", "b.ora": "IFILE = tnsnames.ora"}, alias: "nsn", err: true},
		{name: "1512 the same entry twice", files: map[string]string{"tnsnames.ora": "nsn = host:7212/service\nsome_other_nsn = some_host/some_service\nnsn = host:7212/service\n"},
			alias: "nsn", host: "host", port: "7212", service: "service"},
		{name: "1513 an entry twice, the later wins", files: map[string]string{"tnsnames.ora": "nsn = host_a:7213/service_a\nsome_other_nsn = some_host/some_service\nnsn = host_b:7213/service_b\n"},
			alias: "nsn", host: "host_b", port: "7213", service: "service_b"},
		{name: "1515 an entry of an included file, the later wins", files: map[string]string{"tnsnames.ora": "nsn = host_a:7215/service_a\nIFILE = inc.ora", "inc.ora": "nsn = host_b:7215/service_b\n"},
			alias: "nsn", host: "host_b", port: "7215", service: "service_b"},
		{name: "1516 a missing IFILE", files: map[string]string{"tnsnames.ora": "IFILE = missing.ora\n"}, alias: "anything", err: true},
		{name: "1517 one file included twice", files: map[string]string{"tnsnames.ora": "IFILE = inc.ora\nanother_nsn = some_host/some_service\nIFILE = inc.ora\n", "inc.ora": "nsn = host:7217/service\n"},
			alias: "nsn", host: "host", port: "7217", service: "service"},
		{name: "1518 one file included by two", files: map[string]string{"tnsnames.ora": "IFILE = a.ora\nanother_nsn = some_host/some_service\nIFILE = b.ora\n", "a.ora": "in_a = some_host/some_service\nIFILE = c.ora\n", "b.ora": "in_b = some_host/some_service\nIFILE = c.ora\n", "c.ora": "nsn = host:7218/service\n"},
			alias: "nsn", host: "host", port: "7218", service: "service"},
		{name: "1519 aliases over lines", files: map[string]string{"tnsnames.ora": "nsn_a,\nnsn_b,\nnsn_c = host:7219/service"},
			alias: "nsn_b", host: "host", port: "7219", service: "service"},
		{name: "1520 a comment in a descriptor", files: map[string]string{"tnsnames.ora": "nsn = \n(DESCRIPTION=\n  (ADDRESS=(PROTOCOL=TCP)(HOST=h)(PORT=7220))\n  (CONNECT_DATA=\n    (SERVICE_NAME=s)\n    # embedded comment\n  )\n)\n"},
			alias: "nsn", host: "h", port: "7220", service: "s"},
		{name: "1521 a comment between aliases", files: map[string]string{"tnsnames.ora": "nsn_1 = tcp://host_1:7221/service_1\n# COMMENT \nnsn_2 = tcp://host_2:7222/service_2\n"},
			alias: "nsn_2", host: "host_2", port: "7222", service: "service_2"},
		{name: "1523 IFILE in double quotes", files: map[string]string{"tnsnames.ora": "nsn_a = host_a:7223/service_a\nifile = \"" + filepath.Join(other, "inc.ora") + "\""},
			alias: "nsn_b", host: "host_b", port: "7208", service: "service_b"},
		{name: "an IPv6 address", files: map[string]string{"tnsnames.ora": "nsn = [::1]:7224/service"},
			alias: "nsn", host: "::1", port: "7224", service: "service"},
		{name: "a host alone", files: map[string]string{"tnsnames.ora": "nsn = host"},
			alias: "nsn", host: "host", port: "1521"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, test.files)
			desc, err := LookupAlias(dir, test.alias)
			if test.err {
				if err == nil {
					t.Fatalf("got %s, want an error", desc)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cfg := New()
			if err := cfg.UpdateDatabaseInfo(desc); err != nil {
				t.Fatalf("%s: %v", desc, err)
			}
			if len(cfg.Servers) != 1 || cfg.Servers[0].Addr != test.host || strconv.Itoa(cfg.Servers[0].Port) != test.port || cfg.ServiceName != test.service {
				t.Errorf("%s: got the servers %+v and the service %q", desc, cfg.Servers, cfg.ServiceName)
			}
		})
	}
}
