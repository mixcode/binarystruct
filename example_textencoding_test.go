// Copyright 2021 github.com/mixcode

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"

	// text encodings
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
)

func ExampleMarshaler_AddTextEncoding() {

	// make a explicit marshaller
	var marshaller = new(binarystruct.Marshaler)
	marshaller.Order = binarystruct.LittleEndian

	// add Japanese Shift-JIS text encoding
	// see "golang.org/x/text/encoding/japanese"
	marshaller.AddTextEncoding("sjis", japanese.ShiftJIS)

	// add UTF-16(little endian with BOM) text encoding
	// see "golang.org/x/text/encoding/unicode"
	marshaller.AddTextEncoding("utf16", unicode.UTF16(unicode.LittleEndian, unicode.UseBOM))

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
	data, err := marshaller.Marshal(&in)
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
	_, err = marshaller.Unmarshal(data, &out)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%v\n", out)

	// Output:
	// Marshaled: 0a 00 82 b1 82 f1 82 c9 82 bf 82 cd 06 00 ff fe e0 5c 3c 4e
	// {こんにちは 峠丼}
}

func ExampleMarshaler_DefaultTextEncoding() {
	var marshaller = new(binarystruct.Marshaler)
	marshaller.Order = binarystruct.LittleEndian

	// Add Shift-JIS text encoding
	marshaller.AddTextEncoding("sjis", japanese.ShiftJIS)

	// Set DefaultTextEncoding to "sjis"
	marshaller.DefaultTextEncoding = "sjis"

	type st struct {
		// No encoding option is specified; fallback to DefaultTextEncoding (Shift-JIS)
		S string `binary:"wstring"`
	}

	in := st{
		S: "こんにちは",
	}

	data, err := marshaller.Marshal(&in)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Marshaled:")
	for _, b := range data {
		fmt.Printf(" %02x", b)
	}
	fmt.Println()

	out := st{}
	_, err = marshaller.Unmarshal(data, &out)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%v\n", out)

	// Output:
	// Marshaled: 0a 00 82 b1 82 f1 82 c9 82 bf 82 cd
	// {こんにちは}
}
