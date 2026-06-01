// Copyright 2021-2026 github.com/mixcode

/*

Package binarystruct is an automatic type-converting binary data marshaller/unmarshaller for Go structs and single values.

Binary data formats are usually tightly packed to save space.
Such data often requires type conversions to be used in the Go language context.
This package handles type conversions between Go data types and binary types of struct fields according to their tags.

# A Quick Example

Each field in the struct is tagged with "binary" tags.
The three integer fields are tagged as 1-byte, 2-bytes, and 4-bytes long, and the Header string is tagged as a 4-byte sequence.
Marshal and Unmarshal functions read the tag and convert each field to the specified binary format.

	strc := struct {
		Header       string `binary:"[4]byte"` // marshaled to 4 bytes
		ValueInt8    int    `binary:"int8"`    // marshaled to single byte
		ValueUint16  int    `binary:"uint16"`  // marshaled to two bytes
		ValueDword32 int    `binary:"dword"`   // marshaled to four bytes
	}{"abcd", 1, 2, 3}

	blob, err := binarystruct.Marshal(strc, binarystruct.BigEndian)
	// marshaled blob will be:
	// { 0x61, 0x62, 0x63, 0x64,
	//   0x01,
	//   0x00, 0x02,
	//   0x00, 0x00, 0x00, 0x03 }

# Overview

Go's built-in "encoding/binary" package is the preferred way to deal with binary data structures.
However, in many real-world use cases (e.g. file formats, network protocols), binary data is tightly packed to save space, requiring frequent manual type conversions (such as reading a 1-byte integer from binary and converting it to Go's natural `int` type).

This package simplifies these typecasting burdens by performing automatic type conversion and range checking between Go types and binary formats as described in struct tags. It is designed for developers who need to read or write structured binary data (such as headers, packets, or records) without writing boilerplate decoding/encoding and conversion code.

*/
package binarystruct
