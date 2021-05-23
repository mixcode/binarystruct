package binarystruct_test

import (
	// "fmt"
	// "os"
	"bytes"
	"reflect"
	"testing"

	bst "github.com/mixcode/binarystruct"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
)

func TestTextEncoding(t *testing.T) {
	var err error

	var ms = new(bst.Marshaller)

	ms.AddTextEncoder("sjis", japanese.ShiftJIS)
	ms.AddTextEncoder("utf16", unicode.UTF16(unicode.LittleEndian, unicode.UseBOM))

	// SJIS
	func() {
		type st struct {
			S string `binary:"wstring,encoding=sjis"`
		}
		in := st{S: "こんにちは峠丼"}
		exp := []byte{
			0x0e, 0x00, 0x82, 0xb1, 0x82, 0xf1, 0x82, 0xc9,
			0x82, 0xbf, 0x82, 0xcd, 0x93, 0xbb, 0x98, 0xa5,
		}
		enc, e := ms.Marshal(&in, bst.LittleEndian)
		if e != nil {
			t.Error(e)
			return
		}
		if !bytes.Equal(enc, exp) {
			t.Errorf("encoded bytes are not equal")
			return
		}
		out := st{}
		_, e = ms.Unmarshal(enc, bst.LittleEndian, &out)
		if e != nil {
			t.Error(e)
			return
		}
		if !reflect.DeepEqual(&in, &out) {
			t.Errorf("decoded string is not equal")
		}
	}()

	// utf-16 (little-endian, with bom)
	func() {
		type st struct {
			S string `binary:"string(32),encoding=utf16"`
		}
		in := st{S: "abcこんにちは峠丼def"}
		exp := []byte{
			0xff, 0xfe, 0x61, 0x00, 0x62, 0x00, 0x63, 0x00,
			0x53, 0x30, 0x93, 0x30, 0x6b, 0x30, 0x61, 0x30,
			0x6f, 0x30, 0xe0, 0x5c, 0x3c, 0x4e, 0x64, 0x00,
			0x65, 0x00, 0x66, 0x00, 0x00, 0x00, 0x00, 0x00,
		}
		enc, e := ms.Marshal(&in, bst.LittleEndian)
		if e != nil {
			t.Error(e)
			return
		}
		if !bytes.Equal(enc, exp) {
			t.Errorf("encoded bytes are not equal")
			return
		}
		out := st{}
		_, e = ms.Unmarshal(enc, bst.LittleEndian, &out)
		if e != nil {
			t.Error(e)
			return
		}
		if !reflect.DeepEqual(&in, &out) {
			t.Errorf("decoded string is not equal")
		}
	}()

	if err != nil {
		t.Error(err)
		// t.Errorf("%s", err)
		// t.Fatal(err)
		// t.Fatalf("%s", err)
	}
}
