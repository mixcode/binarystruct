package binarystruct

import (
	"bytes"
	"fmt"
	"testing"
)

func TestWrite(t *testing.T) {
	var err error

	s := struct {
		U1 int    `binary:"uint8"`
		I2 int    `binary:"int32"`
		I3 bool   `binary:"int8"`
		N4 string `binary:"wstring"`
		A5 []byte
		S5 struct {
			F1 float32
			I2 int32
		}
	}{1, 2, true,
		"hello",
		[]byte{1, 2, 3, 4, 5},
		struct {
			F1 float32
			I2 int32
		}{12.34, 0x01020304}}

	//s := struct{ b []byte }{[]byte{1, 2, 3}}

	fmt.Println(s)

	var buf bytes.Buffer
	sz, err := Write(&buf, LittleEndian, &s)
	if err != nil {
		t.Error(err)
	}
	if sz != len(buf.Bytes()) {
		t.Errorf("invalid write size")
	}
	fmt.Printf("%v (%d)", buf.Bytes(), sz)

	var buf2 bytes.Buffer
	sz, err = Write(&buf2, BigEndian, &s)
	if err != nil {
		t.Error(err)
	}
	if sz != len(buf2.Bytes()) {
		t.Errorf("invalid write size")
	}
	fmt.Printf("%v (%d)", buf2.Bytes(), sz)
}
