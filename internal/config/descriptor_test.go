package config

import (
	"fmt"
	"testing"
)

// TestDescriptors reads connect descriptors that python-oracledb tests, in
// tests/misc/test_1300_connect_params.py, written again here. Each names the
// number of its test there.
func TestDescriptors(t *testing.T) {
	tests := []struct {
		name, desc        string
		servers           string
		service, instance string
	}{
		{"1307 retry count and delay",
			"(DESCRIPTION=(RETRY_COUNT=6)(RETRY_DELAY=5)\n(ADDRESS=(PROTOCOL=TCP)(HOST=my_host9)(PORT=1593))\n(CONNECT_DATA=(SERVICE_NAME=my_service_name9)))",
			"TCP:my_host9:1593", "my_service_name9", ""},
		{"1319 spaces and newlines",
			"(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP) \n(HOST=my_host23)\n\n(PORT=1560))(CONNECT_DATA=  (SERVICE_NAME=my_service_name23))\n(SECURITY=(MY_WALLET_DIRECTORY=\"my wallet dir 23\")))",
			"TCP:my_host23:1560", "my_service_name23", ""},
		{"1321 an address list",
			"(DESCRIPTION=(LOAD_BALANCE=ON)(RETRY_COUNT=5)(RETRY_DELAY=2)(ADDRESS_LIST=(LOAD_BALANCE=ON)(ADDRESS=(PROTOCOL=tcp)(HOST=my_host25)(PORT=1321))(ADDRESS=(PROTOCOL=tcp)(HOST=my_host26)(PORT=1322)))(CONNECT_DATA=(SERVICE_NAME=my_service_name25)))",
			"tcp:my_host25:1321 tcp:my_host26:1322", "my_service_name25", ""},
		{"1322 two address lists, HOST after PORT",
			"(DESCRIPTION=(LOAD_BALANCE=ON)(RETRY_COUNT=5)(RETRY_DELAY=2)\n(ADDRESS_LIST=(LOAD_BALANCE=ON)\n(ADDRESS=(PROTOCOL=tcp)(PORT=1521)(HOST=my_host26))\n(ADDRESS=(PROTOCOL=tcp)(PORT=222)(HOST=my_host27)))\n(ADDRESS_LIST=(LOAD_BALANCE=ON)\n(ADDRESS=(PROTOCOL=tcps)(PORT=5555)(HOST=my_host28))\n(ADDRESS=(PROTOCOL=tcps)(PORT=444)(HOST=my_host29)))\n(CONNECT_DATA=(SERVICE_NAME=my_service_name26)))",
			"tcp:my_host26:1521 tcp:my_host27:222 tcps:my_host28:5555 tcps:my_host29:444", "my_service_name26", ""},
		{"1324 an address with an HTTPS proxy",
			"(DESCRIPTION=(ADDRESS=(HTTPS_PROXY=proxy_1324)(HTTPS_PROXY_PORT=8080)(PROTOCOL=tcps)(HOST=my_host1324)(PORT=1324))(CONNECT_DATA=(SERVICE_NAME=s1324)))",
			"tcps:my_host1324:1324", "s1324", ""},
		{"1329 containers, the small one first",
			"(DESCRIPTION=\n(ADDRESS=(PROTOCOL=tcp)(HOST=host1)(PORT=1521))\n(ADDRESS_LIST=\n(ADDRESS=(PROTOCOL=tcp)(HOST=host2a)(PORT=1522))\n(ADDRESS=(PROTOCOL=tcp)(HOST=host2b)(PORT=1523)))\n(ADDRESS=(PROTOCOL=tcp)(HOST=host3)(PORT=1524))\n(CONNECT_DATA=(SERVICE_NAME=my_service_34)))",
			"tcp:host1:1521 tcp:host2a:1522 tcp:host2b:1523 tcp:host3:1524", "my_service_34", ""},
		{"1340 an address with an SDU",
			"(DESCRIPTION=(SDU=65535)(ADDRESS=(PROTOCOL=TCP)(HOST=h1340)(PORT=1340)(SEND_BUF_SIZE=65535))(CONNECT_DATA=(SERVICE_NAME=s1340)))",
			"TCP:h1340:1340", "s1340", ""},
		{"1347 an IPv6 address",
			"(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=::1)(PORT=1347))(CONNECT_DATA=(SERVICE_NAME=s1347)))",
			"TCP:::1:1347", "s1347", ""},
		{"1371 an instance name",
			"(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=host_1371)(PORT=1371))(CONNECT_DATA=(SERVICE_NAME=service_1371)(INSTANCE_NAME=instance_1371)))",
			"tcp:host_1371:1371", "service_1371", "instance_1371"},
		{"lower case keys",
			"(description=(address=(protocol=tcp)(host=lower)(port=1))(connect_data=(service_name=svc)))",
			"tcp:lower:1", "svc", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := New()
			if err := info.UpdateDatabaseInfo(test.desc); err != nil {
				t.Fatal(err)
			}
			servers := ""
			for i, s := range info.Servers {
				if i > 0 {
					servers += " "
				}
				servers += fmt.Sprintf("%s:%s:%d", s.Protocol, s.Addr, s.Port)
			}
			if servers != test.servers || info.ServiceName != test.service || info.InstanceName != test.instance {
				t.Errorf("got %q, the service %q and the instance %q", servers, info.ServiceName, info.InstanceName)
			}
		})
	}
}
