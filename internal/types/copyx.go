package types

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

var truthy = []string{"true", "1"}

func setNil(dest reflect.Value) error {
	if dest.Kind() == reflect.Pointer && dest.IsNil() {
		return nil
	}
	// if the value is support scanner interface call it
	processed, err := copyWithScanner(dest, nil)
	if processed {
		return err
	}
	dest.Set(reflect.Zero(dest.Type()))
	return nil
}

func copyTime(dest reflect.Value, src time.Time) error {
	switch dest.Type() {
	case TyString:
		dest.SetString(src.Format(time.RFC3339))
	case TyTime:
		dest.Set(reflect.ValueOf(src))
	default:
		return defaultCopy(dest, src)
	}
	return nil
}

func copyBytes(dest reflect.Value, src []byte) error {
	switch dest.Type() {
	case TyString:
		dest.SetString(string(src))
	case TyBytes:
		dest.SetBytes(src)
	default:
		return defaultCopy(dest, src)
	}
	return nil
}
func copyString(dest reflect.Value, src string) error {
	var intErr, floatErr, timeErr error
	tempInt, err := strconv.ParseInt(src, 10, 64)
	if err != nil {
		intErr = fmt.Errorf(`can't assign string "%v" to int variable`, src)
	}
	tempFloat, err := strconv.ParseFloat(src, 64)
	if err != nil {
		floatErr = fmt.Errorf(`can't assign string "%v" to float variable`, src)
	}
	tempTime, err := time.Parse(time.RFC3339, src)
	if err != nil {
		timeErr = fmt.Errorf(`can't assign string "%v" to time.Time variable`, src)
	}

	if tSigned(dest.Type()) {
		if intErr == nil {
			dest.SetInt(tempInt)
		}
		return intErr
	}
	if tUnsigned(dest.Type()) {
		if intErr == nil {
			dest.SetUint(uint64(tempInt))
		}
		return intErr
	}
	if tFloat(dest.Type()) {
		if floatErr == nil {
			dest.SetFloat(tempFloat)
		}
		return floatErr
	}
	switch dest.Type() {
	case TyBool:
		if slices.Contains(truthy, strings.ToLower(src)) {
			dest.SetBool(true)
		} else {
			dest.SetBool(false)
		}
	case TyString:
		dest.SetString(src)
	case TyTime:
		if timeErr == nil {
			dest.Set(reflect.ValueOf(tempTime))
		}
		return timeErr
	default:
		return defaultCopy(dest, src)
	}
	return nil
}

func copyArray(dest reflect.Value, src []any) error {
	if dest.Kind() != reflect.Slice {
		return defaultCopy(dest, src)
	}
	var err error
	tempSlice := reflect.MakeSlice(dest.Type(), len(src), len(src))
	for index, item := range src {
		err = RCopy(tempSlice.Index(index), item)
		if err != nil {
			return err
		}
	}
	dest.Set(tempSlice)
	return nil
}
