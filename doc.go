// Copyright 2021 mixcode@github

/*

Package binarystruct is an automatic type-converting binary data marshaller/unmarshaller for go structs.

Binary data formats are usually tightly packed to save spaces.
Such data often require type conversions to be used in the Go language context.
This package handles type conversions between Go data types and binary types of struct fields according to their tags.

For example, assume we have a struct like below.
The three integer fields are marshaled as 1-byte, 2-bytes, and 4-bytes according to their "binary" tag names.
Strings also may be mapped to arrays accordingly.
	// a quick example
	strc := struct {
		Header       string `binary:"[4]byte"` // marshaled to 4 bytes
		ValueInt8    int    `binary:"int8"`    // marshaled to single byte
		ValueUint16  int    `binary:"uint16"`  // marshaled to two bytes
		ValueDword32 int    `binary:"dword"`   // marshaled to four bytes
	}{"abcd", 1, 2, 3}

	blob, err := binarystruct.Marshal(strc, binarystruct.BigEndian)
	// marshaled blob will be
	// { 0x61, 0x62, 0x63, 0x64,
	//   0x01,
	//   0x00, 0x02,
	//   0x00, 0x00, 0x00, 0x03 }

*/
package binarystruct
