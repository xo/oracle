package integration

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/xo/oracle"
)

// sameJSON reports whether the JSON texts a and b hold the same value.
func sameJSON(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// jsonDocs are the documents of the tests of python-oracledb, less the one of
// bytes, which JSON text cannot hold.
var jsonDocs = []string{
	`true`,
	`false`,
	`"String"`,
	`{}`,
	`{"name":null}`,
	`{"name":"John"}`,
	`{"age":30}`,
	`{"Permanent":true}`,
	`{"employee":{"name":"John","age":30,"city":"Delhi","Parmanent":true}}`,
	`{"employees":["John","Matthew","James"]}`,
	`{"employees":[{"employee1":{"name":"John","city":"Delhi"}},{"employee2":{"name":"Matthew","city":"Mumbai"}},{"employee3":{"name":"James","city":"Bangalore"}}]}`,
}

// TestJSONValues binds and reads JSON documents and scalars, alone, in a
// batch, through RETURNING and in queries of SQL/JSON. It holds the cases of
// tests/data_types/test_3100_json.py of python-oracledb, written again here.
// The driver binds JSON as text, so the cases that bind a date or bytes into
// JSON are left out.
func TestJSONValues(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 21)
	lobTable(t, db, "go_json", "id NUMBER, j JSON")
	newJSON := func(text string) *oracle.JSON {
		j, err := oracle.NewJSON(text)
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	// read returns the documents of the table in the order of id
	read := func(query string) []string {
		t.Helper()
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s sql.Null[string]
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			if s.Valid {
				out = append(out, s.V)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	insert := func() {
		t.Helper()
		if _, err := db.Exec("DELETE FROM go_json"); err != nil {
			t.Fatal(err)
		}
		ids := make([]int, len(jsonDocs))
		docs := make([]any, len(jsonDocs))
		for i, doc := range jsonDocs {
			ids[i], docs[i] = i, newJSON(doc)
		}
		if _, err := db.Exec("INSERT INTO go_json VALUES (:1, :2)", ids, docs); err != nil {
			t.Fatal(err)
		}
	}

	// 3100 and 3101 one row, and RETURNING
	for i, doc := range jsonDocs {
		var ret oracle.JSON
		if _, err := db.Exec("INSERT INTO go_json VALUES (:1, :2) RETURNING j INTO :3", i, newJSON(doc), oracle.Out{Dest: &ret}); err != nil {
			t.Errorf("3101 %s: %v", doc, err)
			continue
		}
		if got := string(ret.Text()); !sameJSON(got, doc) {
			t.Errorf("3101 %s: returned %s", doc, got)
		}
		var got string
		if err := db.QueryRow("SELECT j FROM go_json WHERE id = :1", i).Scan(&got); err != nil || !sameJSON(got, doc) {
			t.Errorf("3100 %s: got %s, %v", doc, got, err)
		}
	}

	// 3102 a batch
	insert()
	for i, got := range read("SELECT j FROM go_json ORDER BY id") {
		if !sameJSON(got, jsonDocs[i]) {
			t.Errorf("3102 row %d: got %s, want %s", i, got, jsonDocs[i])
		}
	}

	// 3104, 3105 and 3107 scalars, where a number keeps every digit
	for _, scalar := range []string{
		`true`, `false`, `"String 1"`, `"A much longer string"`, `"Y"`,
		`0`, `1`, `25.25`, `6088343244`, `-9999999999999999999`, `0.25`, `10.25`, `319438950232418390.273596`,
	} {
		var ret oracle.JSON
		if _, err := db.Exec("INSERT INTO go_json VALUES (100, :1) RETURNING j INTO :2", newJSON(scalar), oracle.Out{Dest: &ret}); err != nil {
			t.Errorf("3104 %s: %v", scalar, err)
		} else if got := string(ret.Text()); got != scalar {
			t.Errorf("3104 %s: returned %s", scalar, got)
		}
	}

	// 3111 an update in a batch
	insert()
	ids := make([]int, len(jsonDocs))
	docs := make([]any, len(jsonDocs))
	for i := range jsonDocs {
		ids[i], docs[i] = i, newJSON(`{"a":`+strconv.Itoa(i)+`}`)
	}
	if _, err := db.Exec("UPDATE go_json SET j = :1 WHERE id = :2", docs, ids); err != nil {
		t.Errorf("3111: %v", err)
	}
	for i, got := range read("SELECT j FROM go_json ORDER BY id") {
		if want := `{"a":` + strconv.Itoa(i) + `}`; got != want {
			t.Errorf("3111 row %d: got %s, want %s", i, got, want)
		}
	}

	// 3112 to 3115 queries of SQL/JSON
	insert()
	if got := read("SELECT JSON_QUERY(j, '$.employees.employee1.name') FROM go_json ORDER BY id"); len(got) != 1 {
		t.Errorf("3112 JSON_QUERY of one: got %v", got)
	}
	if got := read("SELECT JSON_QUERY(j, '$.employees') FROM go_json ORDER BY id"); len(got) != 2 {
		t.Errorf("3112 JSON_QUERY of two: got %v", got)
	}
	for path, want := range map[string]int{"$.Permanent": 1, "$.employees": 2} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM go_json WHERE JSON_EXISTS(j, '" + path + "')").Scan(&n); err != nil || n != want {
			t.Errorf("3113 %s: got %d, %v", path, n, err)
		}
	}
	got := read("SELECT t.j.employees FROM go_json t WHERE t.j.employees IS NOT NULL ORDER BY t.id")
	if want := []string{`["John","Matthew","James"]`, `[{"employee1":{"city":"Delhi","name":"John"}},{"employee2":{"city":"Mumbai","name":"Matthew"}},{"employee3":{"city":"Bangalore","name":"James"}}]`}; len(got) != 2 || !sameJSON(got[0], want[0]) || !sameJSON(got[1], want[1]) {
		t.Errorf("3114 the dot notation: got %v", got)
	}
	if _, err := db.Exec("DELETE FROM go_json"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO go_json VALUES (:1, :2)", []int{1, 2, 3}, []any{newJSON(`{"a":12.5}`), newJSON(`{"b":true}`), newJSON(`{"c":null}`)}); err != nil {
		t.Fatal(err)
	}
	if got, want := read("SELECT JSON_SERIALIZE(j) FROM go_json ORDER BY id"), []string{`{"a":12.5}`, `{"b":true}`, `{"c":null}`}; !reflect.DeepEqual(got, want) {
		t.Errorf("3115 JSON_SERIALIZE: got %v", got)
	}
}

// TestJSONTypes reads each scalar type of OSON, as a document and alone. A
// date, an interval and a RAW read as the text that JSON_SERIALIZE writes for
// them, which the test reads from the server too. go-ora read none of them,
// read an interval as a NUMBER, and read no integer of a short form. It holds
// the cases 3109 and 3110 of tests/data_types/test_3100_json.py of
// python-oracledb, written again here.
func TestJSONTypes(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 21)
	const doc = `{
		"binary_float": {"$numberFloat": 38.75},
		"binary_double": {"$numberDouble": 125.875},
		"date_no_time": {"$oracleDate": "2022-12-05"},
		"date_with_time": {"$oracleDate": "2022-12-05T15:06:05"},
		"empty_string": "",
		"explicit_long": {"$numberLong": 9223372036854775807},
		"false": false,
		"interval_ds": {"$intervalDaySecond" : "P133DT2H5M8.123S"},
		"long_integer": 12345678901234567890123456789012345,
		"null": null,
		"short_decimal": {"$numberDecimal": 18.25},
		"short_integer": {"$numberInt": 5 },
		"short_raw": {"$rawhex": "73686f72745f726177"},
		"short_string": "A short string",
		"small_integer": 1234,
		"small_float": 25.25,
		"string_uint8": "A longer string but still < 256 bytes",
		"true": true,
		"ts_no_fs": {"$oracleTimestamp": "2022-12-06T18:12:35"},
		"ts_tz": {"$oracleTimestampTZ": "2022-12-07T22:59:15.1234Z"},
		"ts_with_fs": {"$oracleTimestamp": "2022-12-06T18:12:35.123"}
	}`
	want := `{"binary_double":125.875,"binary_float":38.75,"date_no_time":"2022-12-05T00:00:00",` +
		`"date_with_time":"2022-12-05T15:06:05","empty_string":"","explicit_long":9223372036854775807,` +
		`"false":false,"interval_ds":"P133DT2H5M8.123000S","long_integer":12345678901234567890123456789012345,` +
		`"null":null,"short_decimal":18.25,"short_integer":5,"short_raw":"73686F72745F726177",` +
		`"short_string":"A short string","small_float":25.25,"small_integer":1234,` +
		`"string_uint8":"A longer string but still < 256 bytes","true":true,"ts_no_fs":"2022-12-06T18:12:35",` +
		`"ts_tz":"2022-12-07T22:59:15.123400Z","ts_with_fs":"2022-12-06T18:12:35.123000"}`
	var got string
	if err := db.QueryRow("SELECT JSON('" + doc + "' EXTENDED) FROM dual").Scan(&got); err != nil || got != want {
		t.Errorf("3110 every type: got %s, %v\nwant %s", got, err, want)
	}
	for _, expr := range []string{
		"TO_YMINTERVAL('8-04')", "TO_YMINTERVAL('-1-02')", "TO_YMINTERVAL('0-00')", "TO_YMINTERVAL('0-04')",
		"TO_DSINTERVAL('8 04:00:00')", "TO_DSINTERVAL('-8 04:00:00.5')", "TO_DSINTERVAL('0 00:00:00')",
		"TO_DSINTERVAL('0 00:00:00.123456789')", "TO_DSINTERVAL('0 00:05:08')",
		"TO_TIMESTAMP_TZ('2022-12-07 22:59:15.123456789 +05:30', 'YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM')",
		"TO_TIMESTAMP_TZ('2022-12-07 22:59:15 -07:00', 'YYYY-MM-DD HH24:MI:SS TZH:TZM')",
		"TO_TIMESTAMP_TZ('2022-12-07 22:59:15 UTC', 'YYYY-MM-DD HH24:MI:SS TZR')",
		"TO_TIMESTAMP('2022-12-07 22:59:15.123456789', 'YYYY-MM-DD HH24:MI:SS.FF9')",
		"DATE '0001-01-01'", "TO_DATE('-4712-01-01', 'SYYYY-MM-DD')",
		"HEXTORAW('0102ABCDEF')", "BINARY_DOUBLE_INFINITY", "-BINARY_DOUBLE_INFINITY", "BINARY_FLOAT_NAN",
		"CAST(0.5 AS BINARY_FLOAT)", "-7", "123456789012", "0", "1e125", "-1e-130",
	} {
		var got, server string
		q := "SELECT JSON(JSON_SCALAR(" + expr + ")), JSON_SERIALIZE(JSON(JSON_SCALAR(" + expr + "))) FROM dual"
		if err := db.QueryRow(q).Scan(&got, &server); err != nil {
			t.Errorf("%s: %v", expr, err)
		} else if got != server && !sameJSON(got, server) {
			t.Errorf("%s: got %s, the server writes %s", expr, got, server)
		}
	}
}
