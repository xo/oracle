package ttc

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
)

func TestGetValue(t *testing.T) {
	var val any
	var err error
	// passing nil
	val, err = getValue(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if val != nil {
		t.Errorf("passing nil expected nil and get %v", val)
	}
	// passing primitive value
	val, err = getValue(20)
	if err != nil {
		t.Error(err)
		return
	}
	if val != 20 {
		t.Errorf("passing nil expected 20 and get %v", val)
	}
	// passing sql.Null* value
	val, err = getValue(sql.NullString{String: "this is a test", Valid: true})
	if err != nil {
		t.Error(err)
		return
	}
	if val != "this is a test" {
		t.Errorf("passing nil expected 'this is a test' and get %v", val)
	}
	// passing primitive pointer
	temp := 30
	val, err = getValue(&temp)
	if err != nil {
		t.Error(err)
		return
	}
	if val != 30 {
		t.Errorf("passing pointer int64 value 30 and get %v", val)
	}
	// passing sqlNull pointer
	val, err = getValue(&sql.NullFloat64{Float64: 5.78, Valid: true})
	if err != nil {
		t.Error(err)
		return
	}
	if val != 5.78 {
		t.Errorf("passing pointer to sql.NullFloat64 with value 5.78 and get %v", val)
	}
}

func TestGetInt(t *testing.T) {
	checkGetInt := func(testedValue any, expectedValue int64) error {
		val, err := getInt(testedValue)
		if err != nil {
			return err
		}
		if val != expectedValue {
			return fmt.Errorf("passing %v to getInt and recieve %v", testedValue, val)
		}
		return nil
	}
	// passing nil
	err := checkGetInt(nil, 0)
	if err != nil {
		t.Error(err)
	}
	// passing float
	err = checkGetInt(3.35, 3)
	if err != nil {
		t.Error(err)
	}
	// passing float32
	err = checkGetInt(float32(5.78), 5)
	if err != nil {
		t.Error(err)
	}
	// passing int
	err = checkGetInt(5, 5)
	if err != nil {
		t.Error(err)
	}
	// passing int64
	err = checkGetInt(int64(7), 7)
	if err != nil {
		t.Error(err)
	}
	// passing *float
	tempFloat := float32(5.78)
	err = checkGetInt(&tempFloat, 5)
	if err != nil {
		t.Error(err)
	}
	// passing *int
	tempInt := 9
	err = checkGetInt(&tempInt, 9)
	if err != nil {
		t.Error(err)
	}
	// passing NullFlat
	err = checkGetInt(sql.NullFloat64{Float64: 5.78, Valid: true}, 5)
	if err != nil {
		t.Error(err)
	}
	// passing NullInt
	err = checkGetInt(sql.NullInt64{Int64: 2, Valid: true}, 2)
	if err != nil {
		t.Error(err)
	}
	// passing *NullFloat
	err = checkGetInt(&sql.NullFloat64{Float64: 8.44, Valid: true}, 8)
	if err != nil {
		t.Error(err)
	}
	// passing *NullInt
	err = checkGetInt(&sql.NullInt64{Int64: 10, Valid: true}, 10)
	if err != nil {
		t.Error(err)
	}
	// passing string
	err = checkGetInt("10", 10)
	if err != nil {
		t.Error(err)
	}
	// passing *string
	tempString := "11"
	err = checkGetInt(&tempString, 11)
	if err != nil {
		t.Error(err)
	}

	// passing NullString
	err = checkGetInt(sql.NullString{String: "12", Valid: true}, 12)
	if err != nil {
		t.Error(err)
	}
	// passing *NullString
	err = checkGetInt(&sql.NullString{String: "13", Valid: true}, 13)
	if err != nil {
		t.Error(err)
	}
}

func TestSetArray(t *testing.T) {
	var array []int
	pars := []ParameterInfo{}
	pars = append(pars, ParameterInfo{oPrimValue: int64(5)})
	pars = append(pars, ParameterInfo{oPrimValue: int64(6)})
	pars = append(pars, ParameterInfo{oPrimValue: int64(7)})
	err := setArray(reflect.ValueOf(&array), pars)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println(array)
}
