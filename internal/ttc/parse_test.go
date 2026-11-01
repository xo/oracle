package ttc

import (
	"errors"
	"slices"
	"testing"

	"github.com/xo/oracle/internal/tns"
)

// TestParseNames holds the cases of tests/cursor/test_2400_sql_parser.py of
// python-oracledb (2400 to 2419), written again here. python-oracledb lists
// each name once, and parseSQL lists it each place it appears.
func TestParseNames(t *testing.T) {
	for _, test := range []struct {
		sql   string
		names []string
	}{
		// 2400, a comment of one line
		{"--begin :value2 := :a + :b + :c +:a +3; end;\n" +
			"begin :value2 := :a + :c +3; end; -- not a :bind_variable", []string{"VALUE2", "A", "C"}},
		// 2401, a comment of more than one line
		{"/*--select * from :a where :a = 1\n" +
			"select * from table_names where :a = 1*/\n" +
			"select :table_name, :value from dual", []string{"TABLE_NAME", "VALUE"}},
		// 2402, strings
		{"begin\n:value := to_date('20021231 12:31:00', :format);\nend;", []string{"VALUE", "FORMAT"}},
		// 2403, division
		{"select :a / :b, :c / :d from dual", []string{"A", "B", "C", "D"}},
		// 2404, a parenthesis first
		{"(select :a from dual) union (select :b from dual)", []string{"A", "B"}},
		// 2405, a colon in a quoted name
		{`select ":test", :a from dual`, []string{"A"}},
		// 2406, a letter that is not ASCII
		{"select :méil$ from dual", []string{"MÉIL$"}},
		// 2407, quoted names of binds
		{`select :"percent%" from dual`, []string{"percent%"}},
		{`select : "q?marks" from dual`, []string{"q?marks"}},
		{`select :"percent%(ens)yah" from dual`, []string{"percent%(ens)yah"}},
		{`select :  "per % cent" from dual`, []string{"per % cent"}},
		{`select :"per cent" from dual`, []string{"per cent"}},
		{`select :"par(ens)" from dual`, []string{"par(ens)"}},
		{`select :"more/slashes" from dual`, []string{"more/slashes"}},
		{`select :"%percent" from dual`, []string{"%percent"}},
		{`select :"/slashes/" from dual`, []string{"/slashes/"}},
		{`select :"1col:on" from dual`, []string{"1col:on"}},
		{`select :"col:ons" from dual`, []string{"col:ons"}},
		{`select :"more :: %colons%" from dual`, []string{"more :: %colons%"}},
		{`select :"spaces % spaces" from dual`, []string{"spaces % spaces"}},
		{`select "col:nns", :"col:ons", :id from dual`, []string{"col:ons", "ID"}},
		// 2408, quoted names and comments
		{`select "/*_value1" + : "VaLue_2" + :"*/3VALUE" from dual`, []string{"VaLue_2", "*/3VALUE"}},
		// 2409, strings with quotes and colons
		{`select '"string_1"', :bind_1, ':string_2' from dual`, []string{"BIND_1"}},
		// 2410, comments that hold /*
		{"select\n/* comment 1 with /* */\n:a,\n/* comment 2 with another /* */\n:b\n" +
			"/* comment 3 * * * / */,\n:c\nfrom dual", []string{"A", "B", "C"}},
		// 2411, q-strings
		{"select :a, q'{This contains ' and \" and : just fine}', :b, " +
			"q'[This contains ' and \" and : just fine]', :c, " +
			"q'<This contains ' and \" and : just fine>', :d, " +
			"q'(This contains ' and \" and : just fine)', :e, " +
			"q'$This contains ' and \" and : just fine$', :f from dual", []string{"A", "B", "C", "D", "E", "F"}},
		// 2412, JSON constants
		{"select json_object('foo':dummy), :bv1, json_object('foo'::bv2), :bv3, " +
			"json { 'key1': 57, 'key2' : 58 }, :bv4 from dual", []string{"BV1", "BV2", "BV3", "BV4"}},
		// 2413, a comment that opens with more than one *
		{"/****--select * from :a where :a = 1\n" +
			"select * from table_names where :a = 1****/\n" +
			"select :table_name, :value from dual", []string{"TABLE_NAME", "VALUE"}},
		// 2415, spaces around :=
		{"begin :value2 :=\n   :a  + :b  +   :c +:a +3; end;\nbegin :value2\n:=\n:a + :c +3; end;",
			[]string{"VALUE2", "A", "B", "C", "A", "VALUE2", "A", "C"}},
		// 2416, quotes in comments
		{"select\n/* ' comment 1 */\n:a,\n/* \"comment \" 2 ' */:b\n/* comment 3 '*/,\n:c\n" +
			"/* comment 4 \"\"*/\nfrom dual", []string{"A", "B", "C"}},
		// 2419, DDL has no binds
		{"create or replace trigger test_cursor_2419_tg\n" +
			"after update of updated_column on test_cursor_2419\nfor each row\nbegin\n" +
			"update test_cursor_2419 set some_other_column = :new.updated_column\n" +
			"where some_other_column = :old.updated_column;\nend;", nil},
		// a line break after a name
		{"select * from t where x = :x\nand y = :y", []string{"X", "Y"}},
		// positions, a national q-string and a doubled quote
		{"select :1, :2, nq'[:x]', 'it''s :y', :3 from dual", []string{"1", "2", "3"}},
	} {
		info, err := parseSQL(test.sql)
		if err != nil {
			t.Errorf("%q: %v", test.sql, err)
			continue
		}
		if !slices.Equal(info.names, test.names) {
			t.Errorf("%q: the names %q, want %q", test.sql, info.names, test.names)
		}
	}
}

// TestParseRefuses holds 2414, 2417 and 2418 of
// tests/cursor/test_2400_sql_parser.py of python-oracledb: a string, a
// q-string or a quoted name with no end is an error.
func TestParseRefuses(t *testing.T) {
	for _, sql := range []string{
		"select q'[something from dual",
		"select 'abc, :a from dual",
		"select q'[abc'], 5 from dual",
		`select "abc, :a from dual`,
		`select :"abc from dual`,
	} {
		if _, err := parseSQL(sql); !errors.Is(err, tns.ErrInvalidValue) {
			t.Errorf("%q: the error %v, want ErrInvalidValue", sql, err)
		}
	}
}

// TestParseKind checks the kind of a statement and whether it returns into
// binds. go-ora found RETURNING only with spaces around the list, so
// RETURNING(id)INTO was a statement with no RETURNING, and the server refused
// its binds with ORA-03146.
func TestParseKind(t *testing.T) {
	for _, test := range []struct {
		sql       string
		kind      StmtType
		returning bool
	}{
		{"select 1 from dual", SELECT, false},
		{"  (select 1 from dual)", SELECT, false},
		{"/* a */ -- b\nwith t as (select 1 from dual) select * from t", SELECT, false},
		{"insert into t values (:1)", DML, false},
		{"insert into t values (:1) returning id into :2", DML, true},
		{"insert into t values (:1) RETURNING(id)INTO :2", DML, true},
		{"update t set a = 1 return a into :1", DML, true},
		{"update t set a = 'returning x into' where b = :1", DML, false},
		{"delete from t /* returning a into */ where b = :1", DML, false},
		{"merge into t using s on (t.a = s.a) when matched then update set b = 1", DML, false},
		{"select a into :1 from t", SELECT, false},
		{"begin null; end;", PLSQL, false},
		{"declare a number; begin null; end;", PLSQL, false},
		{"call p(:1)", PLSQL, false},
		{"create table t (a number)", OTHERS, false},
		{"alter session set nls_date_format = 'YYYY'", OTHERS, false},
		{"explain plan for select 1 from dual", OTHERS, false},
	} {
		info, err := parseSQL(test.sql)
		if err != nil {
			t.Errorf("%q: %v", test.sql, err)
			continue
		}
		if info.kind != test.kind || info.returning != test.returning {
			t.Errorf("%q: the kind %d and returning %t, want %d and %t", test.sql, info.kind, info.returning, test.kind, test.returning)
		}
	}
}

// TestParseQuoted checks that parseSQL marks a name in quotes, which keeps
// its case, as 1836 and 1837 of tests/cursor/test_1800_other.py of
// python-oracledb need.
func TestParseQuoted(t *testing.T) {
	info, err := parseSQL(`select :test, :"test", : "Te St", :1 from dual`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"TEST", "test", "Te St", "1"}; !slices.Equal(info.names, want) {
		t.Errorf("the names %q, want %q", info.names, want)
	}
	if want := []bool{false, true, true, false}; !slices.Equal(info.quoted, want) {
		t.Errorf("quoted %v, want %v", info.quoted, want)
	}
}
