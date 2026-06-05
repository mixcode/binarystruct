// Copyright 2021-2026 github.com/mixcode

/*
Package binarystruct is an automatic type-converting binary data marshaller/unmarshaller for Go structs and single values.

Go's built-in binary encoding package, "encoding/binary" is the preferred method to deal with binary data structures. The binary package is quite easy to use, but some cases require additional type conversions when values are tightly packed.
For example, an integer value in raw binary structure may be stored as a word or a byte, but the decoded value would be type-casted to an architecture-dependent integer for easy of use in the Go context.

This package simplifies these typecasting burdens by automatically handling conversion of struct fields using field tags.

AI agents: read https://raw.githubusercontent.com/mixcode/binarystruct/main/llms-full.txt first (see "For AI Agents and Code Assistants" at the end of this document).

# A Quick Example

Assume we have a binary data structure with a magic header and three integers, byte, word, dword each.
By writing binary data types to field tags in Go struct definition, the values are automatically recognized and converted to proper encoding types.

	strc := struct {
		_            struct{} `binary:"endian=big"` // declares this struct's byte order
		Header       string   `binary:"[4]byte"`    // maps to 4 bytes
		ValueInt8    int      `binary:"int8"`       // maps to single signed byte
		ValueUint16  int      `binary:"uint16"`     // maps to two bytes
		ValueDword32 int      `binary:"dword"`      // maps to four bytes
	}{Header: "abcd", ValueInt8: 1, ValueUint16: 2, ValueDword32: 3}

	// Marshal a struct to []byte (no order argument; the struct declares it)
	blob, err := binarystruct.Marshal(&strc)

	// Unmarshal binary data back into the struct
	readsz, err := binarystruct.Unmarshal(blob, &strc)

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
  - custom: Custom codec override (must be paired with codec option).

## Tag Options

  - encoding=NAME: Sets string text encoding (e.g. shift-jis, utf-8).
  - endian=big|little|inverse: Per-field override of the struct's declared byte order (a blank "_ struct{}" field tagged endian= declares the struct's overall order). Use only on fields that differ from it (e.g. mixed-endian formats); inverse flips the inherited order. Do not tag every field.
  - codec=NAME: Applies a registered Codec for custom encoding.
  - omittable: Suppresses EOF errors at this field's start.
  - omittable=Expr: Skips the field if byte size limits are reached.
  - range=min..max: Performs range validation check on integers and floats.
  - match=pattern: Performs regex match validation check on string fields.
  - valueof=Expr: (encode-only) Auto-computes an integer field's serialized value from other fields via bytelen()/count() and arithmetic. Emit-only: the Go field is not modified. See "Computed Field Values" below.
  - const=Value: (encode+decode) Emits a fixed value on encode and validates it on decode (magic numbers/signatures). Integer target uses an integer expression (endian-sensitive); byte-sequence target ([N]byte/string(N)) uses a natural-order hex blob. See "Fixed and Magic Values" below.

## Array and Size Expressions

  - [len]TYPE: Specifies an array whose length is the expression len.
  - TYPE(buf_len): Limits/pads a string or padding buffer to the expression buf_len.

Wherever a tag takes a size or computed value — [len], (buf_len), omittable=Expr, and valueof=Expr — it accepts an expression, not just a literal. Operands are integer literals (decimal, hex 0x1F, octal 0o17, binary 0b1010, with optional _ digit separators) and references to other struct fields; operators are +, -, *, /, and parentheses. Decode-side expressions ([len], (buf_len), omittable) may reference only fields defined before the target; valueof may reference any field. The bytelen()/count() functions are available only inside valueof (see "Computed Field Values").

	type Packet struct {
		HeaderSize  int    `binary:"uint8"`
		PayloadSize int    `binary:"uint16"`
		Payload     []byte `binary:"[PayloadSize - HeaderSize]byte"`
		Tail        []byte `binary:"[0x10]byte"` // fixed 16-byte field via a hex constant
	}

# Computed Field Values

The valueof option computes an integer field's serialized value from other fields at encode time, so a length field need not be filled in by hand:

	type Record struct {
		NameLen uint16 `binary:"uint16,valueof=bytelen(Name)"` // encode: written as len(Name)
		Name    []byte `binary:"[NameLen]byte"`                 // decode: sized from NameLen
	}

valueof expressions may use the functions bytelen(F) (total encoded byte length of any field F) and count(F) (element count of an array or slice field F; not valid for strings — use bytelen for a string's byte length), combined with +, -, *, / and parentheses. valueof is evaluated only when encoding; on decode the field is read normally. It is emit-only: the computed value is written to the stream but the Go field is not modified (encoding stays a pure read). To obtain the computed values in Go, perform a Marshal/Unmarshal round trip.

valueof only derives field lengths and counts. Other derived values, such as CRC checksums, compressed sizes, or offsets, are not computed for you and must be assigned normally.

# Fixed and Magic Values

The const option pins a field to a fixed value: it is emitted on encode (the Go field's value is ignored) and validated on decode, returning an ErrValidationError on mismatch. It is the natural way to express format signatures and version markers:

	type ZIPLocalHeader struct {
		Signature uint32  `binary:"uint32,const=0x04034b50,endian=little"` // 'PK\x03\x04'
		Magic     [8]byte `binary:"[8]byte,const=0x89504e470d0a1a0a"`       // byte-sequence form
	}

An integer const (const=0x04034b50) takes a constant integer expression and is written through the field's byte order, so its bytes depend on endianness; add an explicit endian=little|big so a signature's bytes are deterministic. A byte-sequence const on a [N]byte or string(N) field takes a natural-order hex blob (each byte is two hex digits) and is endianness-independent — PK\x03\x04 is simply const=0x504b0304. const targets must be an integer/bitmap or a raw byte sequence, cannot be combined with valueof, and (for the byte form) require a fixed size matching the constant's length.

# Interface & Polymorphic Handling

binarystruct can serialize and deserialize fields of interface types (e.g., interface{} / any) using two distinct strategies:

## Pre-assigned Interfaces (Static Type Resolution)

If a struct field is of an interface type, the decoder checks if the field has been pre-assigned with a concrete value before Unmarshal is called. If pre-assigned, the decoder automatically resolves the underlying concrete type:

	type Packet struct {
		Payload interface{} `binary:"any"`
	}

	// Pre-assign the interface with the concrete structure
	var data int32 = 0
	pkt := Packet{Payload: &data}

	// Unmarshal decodes binary bytes directly into the 'data' variable
	_, err := binarystruct.NewMarshalerOrder(binarystruct.LittleEndian).Unmarshal(blob, &pkt)

## Dynamic Allocation using a Custom Codec

For packets containing polymorphic payloads (e.g. TLV or packet headers followed by dynamic bodies), you can use a custom Codec. The custom decodec can inspect previously decoded fields of the parent struct and dynamically allocate the appropriate concrete type at runtime:

	type Packet struct {
		MsgType uint8       `binary:"uint8"`
		Payload interface{} `binary:"custom,codec=DynamicPayload"`
	}

	func (s *DynamicPayloadCodec) Decode(r io.Reader, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (value interface{}, n int, err error) {
		// Inspect the previously decoded "MsgType" field in the parent struct
		msgTypeField := parentStruct.FieldByName("MsgType")

		// Allocate the appropriate structure dynamically
		var payload interface{}
		switch msgTypeField.Uint() {
		case 1:
			payload = &MessageA{}
		case 2:
			payload = &MessageB{}
		}

		// Decode binary stream into the allocated structure
		n, err = binarystruct.NewMarshalerOrder(order).Read(r, payload)
		return payload, n, err
	}

# Optional & Omittable Fields

binarystruct allows trailing fields of a struct to be optional via the "omittable" option.

## EOF-based Omission

If "omittable" is specified without an expression, reading the field will silently stop without error if the input stream reaches EOF at the beginning of the field. Pointers or interface fields that are nil will be omitted during serialization.

	type Packet struct {
		Required uint16
		Optional uint32 `binary:"uint32,omittable"`
	}

## Expression-based Omission

If "omittable=Expression" is specified, the field is omitted if the current byte count processed (n) is greater than or equal to the evaluated expression value.

	type Packet struct {
		TotalSize uint16
		Val1      uint32
		Extra1    uint32  `binary:"uint32,omittable=TotalSize"`
		Extra2    *uint32 `binary:"uint32,omittable=TotalSize"`
	}

# JSON Layout Export

The compiled struct layout metadata can be exported as a formatted JSON document by calling ToJSON on the StructLayout:

	js, err := layout.ToJSON()

# Detailed Error Reporting with Byte Offset

When unmarshalling fails, errors are returned as a DecodeError pointer which contains the byte Offset and Field path of the failure:

	_, err := binarystruct.NewMarshalerOrder(order).Unmarshal(corrupted, &pkt)
	if err != nil {
		var decodeErr *binarystruct.DecodeError
		if errors.As(err, &decodeErr) {
			// inspect decodeErr.Offset and decodeErr.Field
		}
	}

# Static Code Generation

To achieve peak performance in production, compile your struct layouts into static Go code using the standalone binarystruct-codegen tool:

	go install github.com/mixcode/binarystruct/binarystruct-codegen@latest
	binarystruct-codegen -type Packet,Header -endian big

The -endian flag (big or little) is required: the generated no-arg MarshalBinary,
UnmarshalBinary, and AppendBinary methods implement the standard library's
order-less encoding interfaces, so the byte order must be chosen explicitly. For
more information, see the README.md file in the binarystruct-codegen package directory.

# For AI Agents and Code Assistants

A comprehensive integration guide written for LLM-based coding agents is
available in the repository as llms-full.txt (indexed by llms.txt):

	https://raw.githubusercontent.com/mixcode/binarystruct/main/llms-full.txt

It covers the tag cheat sheet, dynamic sizing, custom codecs, text
encodings, validation, and common pitfalls. Agents generating code against this
package should read that guide first.

Byte order: a struct declares its byte order with a blank "_ struct{}" field
tagged binary:"endian=big|little" (or by embedding a struct that declares one),
so Marshal, Unmarshal, Write, Read, Append, and Inspect take no order argument.
Order resolution, most specific first: a per-field endian= tag, then the struct's
declaration, then the Marshaler's Order field (a fallback set via
NewMarshalerOrder), and otherwise encoding/decoding a multi-byte value fails loud.
A struct's declaration wins over the Marshaler's fallback. For a value that
declares no order (a bare scalar, a third-party struct), use
NewMarshalerOrder(order); NewMarshaler() supplies no fallback.

Implementing the stdlib encoding interfaces: a tagged type can implement
encoding.BinaryMarshaler/BinaryUnmarshaler/BinaryAppender by delegating to
binarystruct. Because Marshal/Write honor encoding.BinaryMarshaler, a hand-written
MarshalBinary must marshal a method-less twin type to avoid infinite recursion;
see the runnable example and llms-full.txt. The codegen tool emits these methods
for you (recursion-safe).
*/
package binarystruct
