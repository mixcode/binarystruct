// Copyright 2021 mixcode@github

//
// An example of marshalling a complex struct to []byte
//
package binarystruct_test

import (
	"fmt"
	"time"

	"github.com/mixcode/binarystruct"
)

type SomeType int

const (
	SomeType0 SomeType = iota
	SomeType1
)

// a complex data structure
type Header struct {
	// Be sure to set double-quotations in tags!
	Magic string `binary:"[4]byte"` // string converted to 4-byte magic header

	// simple values
	Serial   int      `binary:"int32"`  // integer converted to signed dword
	UnixTime int64    `binary:"uint32"` // int64 converted to unsigned dword
	Type     SomeType `binary:"word"`   // 2-byte word
	Flags    uint32   // if no tag is given, then the size will be its natural size (in this case 4-bytes)

	// both From and To will be converted to int16
	From, To int `binary:"int16"`

	// floating point values
	F1 float32 `binary:"float64"`
	F2 float64 `binary:"int16"` // float types could be converted to integer

	// fixed size data
	Name   *string `binary:"string(0x10)"` // fixed size (16-bytes long) string
	Values []int   `binary:"[0x08]byte"`   // fixed size byte array

	// variable length data
	KeySize int    `binary:"uint16"`
	Key     []byte `binary:"[KeySize]byte"`
	// array size and string length may be sums or subs of integer values and other struct fields

	StrBufLen int      `binary:"uint16"`
	Strings   []string `binary:"[4]string(StrBufLen+1)"` // array of fixed sized strings
}

// A complex example
func Example() {
	// build a struct
	name := "max 16 chars"
	timestamp, _ := time.Parse("2006-01-02", "1980-01-02")
	header := Header{
		// simple values
		Magic:    "HEAD",
		Serial:   0x01000002,
		UnixTime: timestamp.Unix(),
		Type:     SomeType1,
		Flags:    0xfffefdfc,
		From:     3, To: 4,

		// note that float type conversions may inaccurate
		F1: 5.0,
		F2: 6.0,

		// fixed length data
		Name:   &name, // pointers and interfaces are dereferced when marshaled
		Values: []int{7, 8, 9, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e},

		// variable length data
		KeySize: 4,
		Key:     []byte{0x0f, 0x10, 0x11, 0x12},

		StrBufLen: 7,
		Strings:   []string{"aa", "bb", "cc", "dd"},
	}

	// marshalling a struct to []byte
	data, err := binarystruct.Marshal(&header, binarystruct.BigEndian)
	if err != nil {
		panic(err)
	}

	// marshaled data will be as follows
	// ---------
	// 00000000  48 45 41 44 01 00 00 02  12 cf f7 80 00 01 ff fe  |HEAD............|
	// 00000010  fd fc 00 03 00 04 40 14  00 00 00 00 00 00 00 06  |......@.........|
	// 00000020  6d 61 78 20 31 36 20 63  68 61 72 73 00 00 00 00  |max 16 chars....|
	// 00000030  07 08 09 0a 0b 0c 0d 0e  00 04 0f 10 11 12 00 07  |................|
	// 00000040  61 61 00 00 00 00 00 00  62 62 00 00 00 00 00 00  |aa......bb......|
	// 00000050  63 63 00 00 00 00 00 00  64 64 00 00 00 00 00 00  |cc......dd......|
	// ---------

	// unmarshalling []byte to struct
	restored := Header{}
	readsz, err := binarystruct.Unmarshal(data, binarystruct.BigEndian, &restored)
	if err != nil {
		panic(err)
	}

	if readsz != len(data) {
		panic(fmt.Errorf("read and write size does not match: read %d, write %d", readsz, len(data)))
	}

	// Output:
}
