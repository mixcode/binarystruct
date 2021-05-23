// Copyright 2021 mixcode@github

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"
)

// An example of slices and arrays.
func Example_arrays() {

	// slices, arrays and strings
	type exampleStruct struct {

		// fixed size array
		I1 []int `binary:"[4]int16"`

		// variable sized array
		N1 int `binary:"uint16"` // a field value
		N2 int `binary:"int8"`   // another field value
		// array size can contain refererences to other fields and simple integer add-subs
		I2 []int `binary:"[N1+N2+2-1]int8"`

		// string
		S1 string `binary:"string(8)"`    // string of buffer size 8
		S2 string `binary:"string(N2+1)"` // string size also can have field reference

		// length-prefixed strings
		BS  string `binary:"bstring"`  // a byte + []byte. the byte contains the string length
		WS  string `binary:"wstring"`  // a word + []byte. the word contains the string length
		DWS string `binary:"dwstring"` // a dword + []byte. the dword contains the string length

		// array of strings
		SA [4]string `binary:"[]string(4)"` // if a fixed size array is given, then the array size may be omitted

		// special case
		SB string `binary:"[4]string(4)"` // only the first string is used. other 3 strings will be ignored
	}

	src := exampleStruct{
		I1: []int{1, 2, 3, 4},

		N1: 1, N2: 2,
		I2: []int{0, 1, 2, 3}, // N1+N2+2-1 elements long

		S1: "abcd",
		S2: "def", // N2+1 byte long

		BS:  "byte_str",
		WS:  "word_str",
		DWS: "dword_str",

		SA: [4]string{"aa", "bb", "cc", "dd"},
		SB: "zz",
	}

	// marshalling a struct to []byte
	data, err := binarystruct.Marshal(&src, binarystruct.LittleEndian)
	if err != nil {
		panic(err)
	}

	// marshalled result:
	// ---
	// +0000  01 00 02 00 03 00 04 00  01 00 02 00 01 02 03 61  |...............a|
	// +0010  62 63 64 00 00 00 00 64  65 66 08 62 79 74 65 5f  |bcd....def.byte_|
	// +0020  73 74 72 08 00 77 6f 72  64 5f 73 74 72 09 00 00  |str..word_str...|
	// +0030  00 64 77 6f 72 64 5f 73  74 72 61 61 00 00 62 62  |.dword_straa..bb|
	// +0040  00 00 63 63 00 00 64 64  00 00 7a 7a 00 00 00 00  |..cc..dd..zz....|
	// +0050  00 00 00 00 00 00 00 00  00 00                    |..........|
	// ---

	// unmarshalling []byte to a struct
	restored := exampleStruct{}
	_, err = binarystruct.Unmarshal(data, binarystruct.LittleEndian, &restored)
	if err != nil {
		panic(err)
	}
	fmt.Println(restored)

	// Output:
	// {[1 2 3 4] 1 2 [0 1 2 3] abcd def byte_str word_str dword_str [aa bb cc dd] zz}
}
