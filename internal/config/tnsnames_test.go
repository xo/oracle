package config

import (
	"errors"
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
