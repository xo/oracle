package integration

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/xo/oracle"
)

// lobTable creates the table name of the columns and drops it when the test
// ends. A VECTOR and a JSON column are SecureFiles LOBs, which need a
// tablespace of automatic segment space management. SYSTEM, the tablespace of
// system on 26ai Free, is not one, so the table goes in USERS then.
func lobTable(t *testing.T, db *sql.DB, name, columns string) {
	t.Helper()
	drop := "DROP TABLE " + name + " PURGE"
	db.Exec(drop)
	create := "CREATE TABLE " + name + " (" + columns + ")"
	if err := execCmd(db, create); err != nil {
		if err := execCmd(db, create+" TABLESPACE USERS"); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { execCmd(db, drop) })
}

// requireErr fails the test when err is not the error of one of the codes.
func requireErr(t *testing.T, name string, err error, codes ...string) {
	t.Helper()
	for _, code := range codes {
		if err != nil && strings.Contains(err.Error(), code) {
			return
		}
	}
	t.Errorf("%s: got %v, want %s", name, err, strings.Join(codes, " or "))
}

// vectorOf returns the VECTOR of v, a slice of elements or a SparseVector.
func vectorOf(t *testing.T, v any) *oracle.Vector {
	t.Helper()
	vec, err := oracle.CreateVector(v)
	if err != nil {
		t.Fatal(err)
	}
	return vec
}

// insertFetch stores in in the column col of the table, reads it back, and
// checks that it reads as want, both into a *oracle.Vector and into a
// pointer to the type of want.
func insertFetch(t *testing.T, db *sql.DB, table, col, name string, in, want any) {
	t.Helper()
	if _, err := db.Exec("DELETE FROM " + table); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf("INSERT INTO %s (id, %s) VALUES (1, :1)", table, col), vectorOf(t, in)); err != nil {
		t.Errorf("%s: %v", name, err)
		return
	}
	query := fmt.Sprintf("SELECT %s FROM %s", col, table)
	var vec oracle.Vector
	if err := db.QueryRow(query).Scan(&vec); err != nil {
		t.Errorf("%s: %v", name, err)
		return
	}
	if got, err := vec.Value(); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %v, %v\nwant %v", name, got, err, want)
	}
	dest := reflect.New(reflect.TypeOf(want))
	if err := db.QueryRow(query).Scan(dest.Interface()); err != nil {
		t.Errorf("%s: into %T: %v", name, dest.Interface(), err)
	} else if got := dest.Elem().Interface(); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: into %T: got %v\nwant %v", name, dest.Interface(), got, want)
	}
}

// convert returns the values of in, a slice of numbers, as a slice of the
// element type of like.
func convert(in, like any) any {
	src := reflect.ValueOf(in)
	dst := reflect.MakeSlice(reflect.TypeOf(like), src.Len(), src.Len())
	for i := range src.Len() {
		dst.Index(i).Set(src.Index(i).Convert(dst.Type().Elem()))
	}
	return dst.Interface()
}

// fill returns a slice of the type of like of n elements of the value v.
func fill(like any, n int, v float64) any {
	dst := reflect.MakeSlice(reflect.TypeOf(like), n, n)
	for i := range n {
		dst.Index(i).Set(reflect.ValueOf(v).Convert(dst.Type().Elem()))
	}
	return dst.Interface()
}

const vectorColumns = `id NUMBER, flex_all VECTOR, flex_type VECTOR(2), flex8 VECTOR(*, INT8),
	flex32 VECTOR(*, FLOAT32), flex64 VECTOR(*, FLOAT64), v8 VECTOR(16, INT8),
	v32 VECTOR(16, FLOAT32), v64 VECTOR(16, FLOAT64)`

// TestVectorColumns binds vectors of each element type into columns of each
// format, where the server converts them, and reads them back. It holds the
// cases of tests/data_types/test_2700_vector.py of python-oracledb, written
// again here.
func TestVectorColumns(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 23)
	lobTable(t, db, "go_vectors", vectorColumns)
	f32 := []float32{1.23, 4.56, -7.89, 10.11, -12.13, 14.15, -16.17, 18.19, -20.21, 9.23, -2.54, 6.5, 4.21, -1.96, 3.54, 2.6}
	f64 := []float64{-0.0936, -0.0055, 0.0029, 0.0087, -0.0291, -0.0028, 0.0089, 0.0061, 0.0249, -0.0063, 0.0011, -0.0201, 0.0158, -0.0046, -0.0096, 0.0124}
	i8 := []int8{-5, 4, -7, 6, -9, 8, -127, 127, 0, -128, 1, 4, -3, 2, -8, 0}
	tests := []struct {
		name, col string
		in, want  any
	}{
		{"2705 float32 into float32", "v32", f32, f32},
		{"2706 float32 into float64", "v64", f32, convert(f32, []float64{})},
		{"2707 float32 into any format", "flex_all", f32, f32},
		{"2708 float64 into float64", "v64", f64, f64},
		{"2709 float64 into float32", "v32", f64, convert(f64, []float32{})},
		{"2710 float64 into any format", "flex_all", f64, f64},
		{"2713 int8 into int8", "v8", i8, i8},
		{"2714 int8 into float32", "v32", i8, convert(i8, []float32{})},
		{"2715 int8 into float64", "v64", i8, convert(i8, []float64{})},
		{"2716 int8 into any format", "flex_all", i8, i8},
		{"2717 float32 into int8", "v8", convert(i8, []float32{}), i8},
		{"2718 float64 into int8", "v8", convert(i8, []float64{}), i8},
		{"2722 float64 of 65533 dimensions", "flex_all", fill([]float64{}, 65533, 2.5), fill([]float64{}, 65533, 2.5)},
		{"2723 float32 of 65533 dimensions", "flex_all", fill([]float32{}, 65533, 2.5), fill([]float32{}, 65533, 2.5)},
		{"2724 int8 of 65533 dimensions", "flex_all", fill([]int8{}, 65533, 2), fill([]int8{}, 65533, 2)},
		{"a float32 zero", "v32", fill([]float32{}, 16, 0), fill([]float32{}, 16, 0)},
	}
	for _, dims := range []int{30, 70, 255, 256, 65534, 65535} {
		for _, like := range []any{[]float32{}, []float64{}, []int8{}} {
			v := fill(like, dims, 1.5)
			if _, ok := like.([]int8); ok {
				v = fill(like, dims, 3)
			}
			tests = append(tests, struct {
				name, col string
				in, want  any
			}{fmt.Sprintf("2725 %T of %d dimensions", like, dims), "flex_all", v, v})
		}
	}
	for _, test := range tests {
		insertFetch(t, db, "go_vectors", test.col, test.name, test.in, test.want)
	}

	for _, n := range []int{4, 20} {
		_, err := db.Exec("INSERT INTO go_vectors (id, v64) VALUES (2, :1)", vectorOf(t, fill([]float64{}, n, 0.625)))
		requireErr(t, fmt.Sprint("2711 ", n, " dimensions into 16"), err, "ORA-51803")
	}
	_, err = db.Exec("INSERT INTO go_vectors (id, v8) VALUES (2, :1)",
		vectorOf(t, []float32{-130, -129, 0, 1, 2, 3, 127, 128, 129, 348, 12, 49, 78, 12, 9, 2}))
	requireErr(t, "2721 float32 past int8", err, "ORA-51806")
	inf := make([]float64, 16)
	for i := range inf {
		inf[i] = math.Inf(i%2*2 - 1)
	}
	var out oracle.Vector
	err = db.QueryRow("SELECT :1 FROM dual", vectorOf(t, inf)).Scan(&out)
	requireErr(t, "2730 infinity", err, "ORA-51805", "ORA-51807")

	// 2719 RETURNING
	in := []float64{6423.5, 6423.625}
	if _, err := db.Exec("DELETE FROM go_vectors"); err != nil {
		t.Fatal(err)
	}
	var ret oracle.Vector
	if _, err := db.Exec("INSERT INTO go_vectors (id, flex_type) VALUES (1, :1) RETURNING flex_type INTO :2", vectorOf(t, in), oracle.Out{Dest: &ret}); err != nil {
		t.Errorf("2719 RETURNING: %v", err)
	} else if got, err := ret.Value(); err != nil || !reflect.DeepEqual(got, in) {
		t.Errorf("2719 RETURNING: got %v, %v", got, err)
	}

	// 2720 NULL
	if _, err := db.Exec("INSERT INTO go_vectors (id) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	var null oracle.Vector
	var nullSlice []float64
	if err := db.QueryRow("SELECT flex_type FROM go_vectors WHERE id = 2").Scan(&null); err != nil {
		t.Errorf("2720 NULL: %v", err)
	} else if got, err := null.Value(); got != nil || err != nil {
		t.Errorf("2720 NULL: got %v, %v", got, err)
	}
	if err := db.QueryRow("SELECT flex_type FROM go_vectors WHERE id = 2").Scan(&nullSlice); err != nil || nullSlice != nil {
		t.Errorf("2720 NULL into a slice: got %v, %v", nullSlice, err)
	}

	// 2734 a batch
	if _, err := db.Exec("DELETE FROM go_vectors"); err != nil {
		t.Fatal(err)
	}
	// a row longer than the last, and a NULL last, which go-ora sent
	// with the locator and the coder of the last row
	batch := []any{vectorOf(t, []float32{1, 2}), vectorOf(t, []float64{3, 4}), vectorOf(t, []int8{5, 6}), nil}
	if _, err := db.Exec("INSERT INTO go_vectors (id, flex_all) VALUES (:1, :2)", []int{1, 2, 3, 4}, batch); err != nil {
		t.Errorf("2734 a batch: %v", err)
	} else {
		for i, want := range []any{[]float32{1, 2}, []float64{3, 4}, []int8{5, 6}} {
			var got oracle.Vector
			if err := db.QueryRow("SELECT flex_all FROM go_vectors WHERE id = :1", i+1).Scan(&got); err != nil {
				t.Errorf("2734 a batch, row %d: %v", i+1, err)
			} else if v, err := got.Value(); err != nil || !reflect.DeepEqual(v, want) {
				t.Errorf("2734 a batch, row %d: got %v, %v, want %v", i+1, v, err, want)
			}
		}
	}

	// 2732 a vector in a JSON value
	var text string
	if err := db.QueryRow("SELECT JSON_OBJECT('id' : 6432, 'vector' : TO_VECTOR('[1, 2, 3]') RETURNING JSON) FROM dual").Scan(&text); err != nil {
		t.Errorf("2732 a vector in JSON: %v", err)
	} else if want := `{"id":6432,"vector":[1,2,3]}`; text != want {
		t.Errorf("2732 a vector in JSON: got %s, want %s", text, want)
	}
}

// TestVectorPLSQL binds vectors in, out and in out of PL/SQL. It holds the
// cases 2745 to 2747 of tests/data_types/test_2700_vector.py of
// python-oracledb, written again here.
func TestVectorPLSQL(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 23)
	const block = `BEGIN
		SELECT VECTOR_DISTANCE(:1, :2, EUCLIDEAN) INTO :3;
		:4 := :2;
		:2 := :1;
	END;`
	tests := []struct {
		name     string
		a, b     any
		distance float64
	}{
		{"2745 float32", []float32{1, 1.5, 2, 2.5}, []float32{4, 4.5, 5, 5.5}, 6},
		{"2745 float32 of 65535", fill([]float32{}, 65535, 3.5), fill([]float32{}, 65535, 2.5), 256},
		{"2746 float64", []float64{1, 1.5, 2, 2.5}, []float64{4, 4.5, 5, 5.5}, 6},
		{"2746 float64 of 65535", fill([]float64{}, 65535, 3.5), fill([]float64{}, 65535, 2.5), 256},
		{"2747 int8", []int8{1, 2, 3, 4}, []int8{5, 6, 7, 8}, 8},
		{"2747 int8 of 65535", fill([]int8{}, 65535, 3), fill([]int8{}, 65535, 2), 256},
	}
	for _, test := range tests {
		inOut := vectorOf(t, test.b)
		var out oracle.Vector
		var distance float64
		_, err := db.Exec(block, vectorOf(t, test.a), oracle.Out{Dest: inOut, In: true}, oracle.Out{Dest: &distance}, oracle.Out{Dest: &out})
		if err != nil {
			t.Errorf("%s: %v", test.name, err)
			continue
		}
		if got, err := out.Value(); err != nil || !reflect.DeepEqual(got, test.b) {
			t.Errorf("%s: out got %v, %v", test.name, got, err)
		}
		if got, err := inOut.Value(); err != nil || !reflect.DeepEqual(got, test.a) {
			t.Errorf("%s: in out got %v, %v", test.name, got, err)
		}
		if math.Abs(distance-test.distance) > 0.01 {
			t.Errorf("%s: distance %v, want %v", test.name, distance, test.distance)
		}
	}
}

// TestBinaryVector binds and reads a VECTOR of the BINARY format, which holds
// 8 dimensions in each byte. It holds the cases of
// tests/data_types/test_2900_binary_vector.py of python-oracledb, written
// again here.
func TestBinaryVector(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 23)
	lobTable(t, db, "go_binary_vectors", "id NUMBER, b VECTOR(64, BINARY)")
	in := []byte{4, 8, 12, 4, 98, 127, 25, 78}
	insertFetch(t, db, "go_binary_vectors", "b", "2900 BINARY", in, in)
	other := []byte{20, 9, 15, 34, 108, 125, 35, 88}
	insertFetch(t, db, "go_binary_vectors", "b", "2902 BINARY", other, other)
	var distance float64
	if err := db.QueryRow("SELECT VECTOR_DISTANCE(b, :1) FROM go_binary_vectors", vectorOf(t, other)).Scan(&distance); err != nil || math.Abs(distance) > 1e-9 {
		t.Errorf("2902 the distance: got %v, %v", distance, err)
	}
	_, err = db.Exec("INSERT INTO go_binary_vectors VALUES (2, :1)", vectorOf(t, []byte{1, 2}))
	requireErr(t, "16 dimensions into 64", err, "ORA-51803")
}

// TestSparseVector binds sparse vectors into dense and sparse columns, where
// the server converts them, and reads them back. It holds the cases of
// tests/data_types/test_3000_sparse_vector.py of python-oracledb, written
// again here.
func TestSparseVector(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 23)
	var supported bool
	db.QueryRow("SELECT COUNT(*) FROM dual WHERE VECTOR('[2, [0], [1]]', 2, FLOAT32, SPARSE) IS NOT NULL").Scan(&supported)
	if !supported {
		t.Skip("sparse vectors need 23.7")
	}
	lobTable(t, db, "go_vectors", vectorColumns)
	lobTable(t, db, "go_sparse_vectors", `id NUMBER, flex_all VECTOR(*, *, SPARSE),
		flex_type VECTOR(2, *, SPARSE), flex8 VECTOR(*, INT8, SPARSE),
		flex32 VECTOR(*, FLOAT32, SPARSE), flex64 VECTOR(*, FLOAT64, SPARSE),
		v8 VECTOR(16, INT8, SPARSE), v32 VECTOR(16, FLOAT32, SPARSE), v64 VECTOR(16, FLOAT64, SPARSE)`)
	sparse := func(dims uint32, values any) oracle.SparseVector {
		return oracle.SparseVector{Dimensions: dims, Indices: []uint32{1, 3, 5}, Values: values}
	}
	// dense returns the dense vector of v, of the element type of like
	dense := func(v oracle.SparseVector, like any) any {
		d := reflect.MakeSlice(reflect.TypeOf(like), int(v.Dimensions), int(v.Dimensions))
		values := reflect.ValueOf(v.Values)
		for i, index := range v.Indices {
			d.Index(int(index)).Set(values.Index(i).Convert(d.Type().Elem()))
		}
		return d.Interface()
	}
	f32, f64 := []float32{1.5, 0.25, 0.5}, []float64{1.5, 0.25, 0.5}
	ints32, ints64, ints8 := []float32{1, 0, 5}, []float64{1, 0, 5}, []int8{1, 0, 5}
	tests := []struct {
		name, col string
		in        oracle.SparseVector
		like      any
	}{
		{"3004 float32 into float32", "v32", sparse(16, f32), []float32{}},
		{"3005 float64 into float64", "v64", sparse(16, f64), []float64{}},
		{"3006 float32 into any format", "flex_all", sparse(16, f32), []float32{}},
		{"3008 float32 into float32", "v32", sparse(16, f32), []float32{}},
		{"3009 float64 into any format", "flex_all", sparse(16, f64), []float64{}},
		{"3012 float32 into int8", "v8", sparse(16, ints32), []int8{}},
		{"3013 float32 of integers into float32", "v32", sparse(16, ints32), []float32{}},
		{"3014 int8 into float64", "v64", sparse(16, ints8), []float64{}},
		{"3015 int8 into any format", "flex_all", sparse(16, ints8), []int8{}},
		{"3017 float64 into int8", "v8", sparse(16, ints64), []int8{}},
		{"3021 float32 of 65533 dimensions", "flex_all", sparse(65533, ints32), []float32{}},
	}
	for _, dims := range []uint32{30, 70, 255, 256, 65534, 65535} {
		for _, values := range []any{[]float32{1.5, 1.5, 1.5}, []float64{1.5, 1.5, 1.5}, []int8{3, 3, 3}} {
			tests = append(tests, struct {
				name, col string
				in        oracle.SparseVector
				like      any
			}{fmt.Sprintf("3022 %T of %d dimensions", values, dims), "flex_all", sparse(dims, values), values})
		}
	}
	for _, test := range tests {
		insertFetch(t, db, "go_vectors", test.col, test.name+", dense", test.in, dense(test.in, test.like))
		want := test.in
		want.Values = convert(test.in.Values, test.like)
		insertFetch(t, db, "go_sparse_vectors", test.col, test.name+", sparse", test.in, want)
	}

	for _, table := range []string{"go_vectors", "go_sparse_vectors"} {
		for _, n := range []uint32{4, 20} {
			_, err := db.Exec("INSERT INTO "+table+" (id, v64) VALUES (2, :1)", oracle.SparseVector{Dimensions: n, Indices: []uint32{2, 3}, Values: []float32{6.54, 9.6}})
			requireErr(t, fmt.Sprint("3010 ", n, " dimensions into 16 of ", table), err, "ORA-51803")
		}
		_, err := db.Exec("INSERT INTO "+table+" (id, v8) VALUES (2, :1)", sparse(16, []float32{-130, 400, 5}))
		requireErr(t, "3020 float32 past int8 into "+table, err, "ORA-51806")
	}
	var out oracle.Vector
	err = db.QueryRow("SELECT :1 FROM dual", sparse(16, []float64{math.Inf(1), math.Inf(-1), 1})).Scan(&out)
	requireErr(t, "3025 infinity", err, "ORA-51805", "ORA-51807")

	// 3018 RETURNING
	in := sparse(16, ints32)
	if _, err := db.Exec("DELETE FROM go_sparse_vectors"); err != nil {
		t.Fatal(err)
	}
	var ret oracle.Vector
	if _, err := db.Exec("INSERT INTO go_sparse_vectors (id, flex32) VALUES (1, :1) RETURNING flex32 INTO :2", in, oracle.Out{Dest: &ret}); err != nil {
		t.Errorf("3018 RETURNING: %v", err)
	} else if got, err := ret.Value(); err != nil || !reflect.DeepEqual(got, in) {
		t.Errorf("3018 RETURNING: got %v, %v", got, err)
	}

	// 3019 NULL
	if _, err := db.Exec("INSERT INTO go_sparse_vectors (id) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	var null *oracle.SparseVector
	if err := db.QueryRow("SELECT flex_type FROM go_sparse_vectors WHERE id = 2").Scan(&null); err != nil || null != nil {
		t.Errorf("3019 NULL: got %v, %v", null, err)
	}

	// 3035 VECTOR() makes a dense or a sparse vector of either
	if _, err := db.Exec("DELETE FROM go_sparse_vectors"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO go_sparse_vectors (id, flex_all) VALUES (1, :1)", oracle.SparseVector{Dimensions: 16, Indices: []uint32{1}, Values: []float32{9}}); err != nil {
		t.Fatal(err)
	}
	for _, format := range []struct {
		name string
		like any
	}{{"INT8", []int8{}}, {"FLOAT32", []float32{}}, {"FLOAT64", []float64{}}} {
		q := fmt.Sprintf("SELECT VECTOR(flex_all, 16, %s, DENSE), VECTOR(flex_all, 16, %s, SPARSE) FROM go_sparse_vectors", format.name, format.name)
		d := reflect.New(reflect.TypeOf(format.like))
		var s oracle.SparseVector
		if err := db.QueryRow(q).Scan(d.Interface(), &s); err != nil {
			t.Errorf("3035 %s: %v", format.name, err)
			continue
		}
		if got := reflect.ValueOf(d.Elem().Interface()); got.Len() != 16 || got.Index(1).Convert(reflect.TypeFor[float64]()).Float() != 9 {
			t.Errorf("3035 %s, dense: got %v", format.name, d.Elem().Interface())
		}
		if reflect.TypeOf(s.Values) != reflect.TypeOf(format.like) || s.Dimensions != 16 || !reflect.DeepEqual(s.Indices, []uint32{1}) {
			t.Errorf("3035 %s, sparse: got %+v", format.name, s)
		}
	}

	// 3036 FROM_VECTOR writes the text of a sparse vector
	if _, err := db.Exec("DELETE FROM go_sparse_vectors"); err != nil {
		t.Fatal(err)
	}
	v := oracle.SparseVector{Dimensions: 16, Indices: []uint32{1, 2, 15}, Values: []float64{2, 45.5, 73.25}}
	if _, err := db.Exec("INSERT INTO go_sparse_vectors (id, v64) VALUES (1, :1)", v); err != nil {
		t.Fatal(err)
	}
	for format, want := range map[string]string{
		"SPARSE": "[16,[1,2,15],[2.0E+000,4.55E+001,7.325E+001]]",
		"DENSE":  "[0,2.0E+000,4.55E+001,0,0,0,0,0,0,0,0,0,0,0,0,7.325E+001]",
	} {
		var text string
		err := db.QueryRow("SELECT FROM_VECTOR(v64 RETURNING VARCHAR2 FORMAT " + format + ") FROM go_sparse_vectors").Scan(&text)
		if err != nil {
			t.Errorf("3036 %s: %v", format, err)
		} else if strings.ReplaceAll(text, " ", "") != want {
			t.Errorf("3036 %s: got %s, want %s", format, text, want)
		}
	}
}
