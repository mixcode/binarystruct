// Copyright 2021 mixcode@github

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"

	// text encoders
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
)

func ExampleMarshaller_AddTextEncoder() {

	// make a explicit marshaller
	var marshaller = new(binarystruct.Marshaller)

	// add Japanese Shift-JIS text encoder
	// see "golang.org/x/text/encoding/japanese"
	marshaller.AddTextEncoder("sjis", japanese.ShiftJIS)

	// add UTF-16(little endian with BOM) text encoder
	// see "golang.org/x/text/encoding/unicode"
	marshaller.AddTextEncoder("utf16", unicode.UTF16(unicode.LittleEndian, unicode.UseBOM))

	type st struct {
		// wstring is []byte prefixed by a word for length
		S string `binary:"wstring,encoding=sjis"`
		T string `binary:"wstring,encoding=utf16"`
	}

	in := st{
		S: "こんにちは", // will be encoded to Shift-JIS
		T: "峠丼",    // will be encoded to UTF-16
	}

	// marshalling
	data, err := marshaller.Marshal(&in, binarystruct.LittleEndian)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Marshaled:")
	for _, b := range data {
		fmt.Printf(" %02x", b)
	}
	fmt.Println()

	// unmarshalling
	out := st{}
	_, err = marshaller.Unmarshal(data, binarystruct.LittleEndian, &out)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%v\n", out)

	// Output:
	// Marshaled: 0a 00 82 b1 82 f1 82 c9 82 bf 82 cd 06 00 ff fe e0 5c 3c 4e
	// {こんにちは 峠丼}
}
