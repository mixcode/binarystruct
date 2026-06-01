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

# Key Features

1. Single-Value Marshalling:
Encode and decode non-struct variables directly using MarshalAs and UnmarshalAs.

	var a []int
	blob, err := binarystruct.MarshalAs(a, "[4]int8", binarystruct.LittleEndian)

2. Explicit Endianness Override:
Specify field-level endianness using `endian=big`, `endian=little`, or `endian=inverse`.

	type Header struct {
		Size   uint32 `binary:"uint32,endian=big"`     // Always BigEndian
		Status uint16 `binary:"uint16,endian=inverse"` // Inverse of active byte order
	}

3. Default Text Encoding:
Configure a fallback default text encoding on the Marshaller instance.

	marshaller := &binarystruct.Marshaller{DefaultTextEncoding: "sjis"}
	marshaller.AddTextEncoding("sjis", japanese.ShiftJIS)

4. Complex Tag Evaluation:
Reference other fields inside array/string sizes using formulas with support for `+`, `-`, `*`, `/`, and parenthesized expressions.

	type Packet struct {
		Length uint8
		Data   []byte `binary:"[(Length*2)+4]byte"`
	}

5. Custom Serializers:
Register and invoke user-defined encoders/decoders for specific fields using `serializer=Name`.

	marshaller.AddSerializer("varint", myVarintSerializer)
	type Record struct {
		Val int `binary:"custom,serializer=varint"`
	}

*/
package binarystruct
