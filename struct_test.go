// Copyright 2021 github.com/mixcode

package binarystruct

import (
	//"fmt"
	// "os"

	"reflect"
	"testing"
)

// test funcion for evaluateTagValue()
func TestEvaulateTagValue(t *testing.T) {

	type testS struct {
		n1 int
		n2 uint8
		s  string
		n3 uint8
	}
	s := testS{1, 2, "hi", 4}
	r := reflect.ValueOf(s)

	testCase := []struct {
		tag   string
		value int
		isErr bool
	}{
		{"3", 3, false},
		{" 4", 4, false},
		{"5 ", 5, false},
		{" 6 ", 6, false},
		{"0x10 + 0o7", 0x17, false},
		{"1-2", -1, false},
		{" -3 + 5", 2, false},
		{"n1", 1, false},
		{" n1 +n2 ", 3, false},
		{" n1 - n3 ", -3, false},
		{" 3*4 ", 0, true}, // add and sub only
		{" 1+s ", 0, true}, // cannot use non-numeric member
		{" 1+u ", 0, true}, // cannot use non-numeric member
	}
	for _, c := range testCase {
		result, err := evaluateTagValue(r, c.tag)
		if !c.isErr && err != nil {
			t.Error(err)
			continue
		}
		if c.isErr && err == nil {
			t.Errorf("must produce an error")
			continue
		}
		if result != c.value {
			t.Errorf("value not match: expected %d, actual %d", c.value, result)
		}
	}

}

func TestArrayPointer(t *testing.T) {
	type subSt struct {
		N int8
	}

	type st struct {
		Count int      `binary:"uint8"`
		A     []*subSt `binary:"[Count]"`
	}

	blob := []byte{0x03, 0x01, 0x02, 0x03}

	var out st
	sz, err := Unmarshal(blob, LittleEndian, &out)
	if err != nil {
		t.Error(err)
	}
	if sz != len(blob) {
		t.Errorf("read size mismatch: expected %d, actual %d", len(blob), sz)
	}
}

func TestMiscStruct(t *testing.T) {

	// Test ignoreing field with unsupported type
	type st struct {
		M     map[string]string `binary:"ignore"`
		Count int               `binary:"uint8"`
	}
	blob := []byte{0x01}

	var out st
	sz, err := Unmarshal(blob, LittleEndian, &out)
	if err != nil {
		t.Error(err)
	}
	if sz != len(blob) {
		t.Errorf("read size mismatch: expected %d, actual %d", len(blob), sz)
	}

}

/*
func TestMisc(t *testing.T) {
	r := regexp.MustCompile(`^\s*(\[([^\]]+)\])?([^\s\(\)]+)(\(([^\)]+)\))?`)
	s := "[7]zstring(0x16)"

	m := r.FindStringSubmatch(s)

	for i, k := range m {
		fmt.Printf("(%d)<%s>\n", i, k)
	}
}
*/
