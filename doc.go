// Copyright 2021-2026 github.com/mixcode

/*
Package binarystruct is an automatic type-converting binary data marshaller/unmarshaller for Go structs and single values.

Go's built-in binary encoding package, "encoding/binary" is the preferred method to deal with binary data structures. The binary package is quite easy to use, but some cases require additional type conversions when values are tightly packed.
For example, an integer value in raw binary structure may be stored as a word or a byte, but the decoded value would be type-casted to an architecture-dependent integer for easy of use in the Go context.

This package simplifies these typecasting burdens by automatically handling conversion of struct fields using field tags.

# A Quick Example

Assume we have a binary data structure with a magic header and three integers, byte, word, dword each.
By writing binary data types to field tags in Go struct definition, the values are automatically recognized and converted to proper encoding types.

	strc := struct {
		Header       string `binary:"[4]byte"` // maps to 4 bytes
		ValueInt8    int    `binary:"int8"`    // maps to single signed byte
		ValueUint16  int    `binary:"uint16"`  // maps to two bytes
		ValueDword32 int    `binary:"dword"`   // maps to four bytes
	}{"abcd", 1, 2, 3}

	// Marshal a struct to []byte
	blob, err := binarystruct.Marshal(&strc, binarystruct.BigEndian)

	// Unmarshal binary data back into the struct
	readsz, err := binarystruct.Unmarshal(blob, binarystruct.BigEndian, &strc)

# Struct Tag Reference

Struct fields can be annotated with the "binary" tag to define their binary layout, type conversions, and size bounds:

	`binary:"[array_len]TYPE(buf_len),option1=val1,option2"`

Example:
	MyString string `binary:"string(StrLen+2),encoding=shift-jis,omittable"`

## Supported Binary Types

  - int8, int16, int32, int64: Signed integers (1, 2, 4, 8 bytes).
  - uint8, uint16, uint32, uint64: Unsigned integers (1, 2, 4, 8 bytes).
  - byte, word, dword, qword: Type-agnostic bitmaps (1, 2, 4, 8 bytes).
  - float32, float64: IEEE 754 floating point values (4, 8 bytes).
  - string: Raw byte string. Padded with 0 up to optional (buf_len).
  - bstring, wstring, dwstring: Length-prefixed string (1, 2, 4 bytes prefix).
  - zstring, z16string: Null-terminated / null-word-terminated strings.
  - pad: Zero-filled padding bytes of (buf_len) size (source value ignored).
  - ignore, -: Ignored field during serialization.
  - any: Natural type encoding (default).
  - custom: Custom serializer override (must be paired with serializer option).

## Tag Options

  - encoding=NAME: Sets string text encoding (e.g. shift-jis, utf-8).
  - endian=big|little|inverse: Overrides default byte order for this field.
  - serializer=NAME: Applies a registered Serializer for custom encoding.
  - omittable: Suppresses EOF errors at this field's start.
  - omittable=Expr: Skips the field if byte size limits are reached.

## Array and Size Expressions

  - [len]TYPE: Specifies an array of the given length.
  - TYPE(buf_len): Limits/pads string or padding buffer size.

Both array length [len] and buffer size (buf_len) can use arithmetic expressions (+, -, *, /, and parentheses) referencing other struct fields:

	type Packet struct {
		HeaderSize  int    `binary:"uint8"`
		PayloadSize int    `binary:"uint16"`
		Payload     []byte `binary:"[PayloadSize - HeaderSize]byte"`
	}
*/
package binarystruct
