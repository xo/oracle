package types

import (
	"testing"
)

func TestCopy(t *testing.T) {
	var v1 string
	err := Copy(&v1, "foo")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(v1)

	var v2 Blob
	err = Copy(&v2, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(v2)
}

func TestCopyStringFromOthers(t *testing.T) {
	var v_string string
	err := Copy(&v_string, 100)
	if err != nil {
		t.Fatal(err)
	}
	if v_string != "100" {
		t.Fatal("error copy int to string")
	}
	err = Copy(&v_string, int64(100))
	if err != nil {
		t.Fatal(err)
	}
	if v_string != "100" {
		t.Fatal("error copy int64 to string")
	}
	err = Copy(&v_string, float64(1.1))
	if err != nil {
		t.Fatal(err)
	}
	if v_string != "1.1" {
		t.Fatal("error copy float64 to string")
	}
	err = Copy(&v_string, true)
	if err != nil {
		t.Fatal(err)
	}
	if v_string != "true" {
		t.Fatal("error copy true to string")
	}
}
