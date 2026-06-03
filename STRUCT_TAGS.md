# Struct Tag Reference Manual

`binarystruct` uses the `binary` struct tag to map Go struct fields to binary data layouts, perform automatic type conversion, and apply dynamic size validation.

---

## 1. Syntax Overview

A struct tag consists of a **binary type** (optionally prefixed by array dimensions or postfixed by buffer size), followed by zero or more comma-separated **options**:

```go
`binary:"[array_len]TYPE(buf_len),option1=val1,option2"`
```

Example:
```go
// A shift-jis string of length defined by the field "StrLen" (plus 2 padding), which is omittable
MyString string `binary:"string(StrLen+2),encoding=shift-jis,omittable"`
```

---

## 2. Binary Types

| Tag Type | Go Kind | Serialized Size | Description |
| :--- | :--- | :--- | :--- |
| **`int8`** | Signed Int / Bool | 1 byte | 8-bit signed integer |
| **`int16`** | Signed Int | 2 bytes | 16-bit signed integer |
| **`int32`** | Signed Int | 4 bytes | 32-bit signed integer |
| **`int64`** | Signed Int | 8 bytes | 64-bit signed integer |
| **`uint8`** | Unsigned Int | 1 byte | 8-bit unsigned integer |
| **`uint16`** | Unsigned Int | 2 bytes | 16-bit unsigned integer |
| **`uint32`** | Unsigned Int | 4 bytes | 32-bit unsigned integer |
| **`uint64`** | Unsigned Int | 8 bytes | 64-bit unsigned integer |
| **`byte`** | Any | 1 byte | Type-agnostic 8-bit bitmap |
| **`word`** | Any | 2 bytes | Type-agnostic 16-bit bitmap |
| **`dword`** | Any | 4 bytes | Type-agnostic 32-bit bitmap |
| **`qword`** | Any | 8 bytes | Type-agnostic 64-bit bitmap |
| **`float32`** | Float | 4 bytes | IEEE 754 32-bit float |
| **`float64`** | Float | 8 bytes | IEEE 754 64-bit float |
| **`string`** | String / Slice | Variable / `buf_len` | Raw byte string (padded with `0` up to `buf_len` if specified) |
| **`bstring`** | String | 1 + len bytes | Length-prefixed string (1 byte length prefix) |
| **`wstring`** | String | 2 + len bytes | Length-prefixed string (2 bytes length prefix) |
| **`dwstring`** | String | 4 + len bytes | Length-prefixed string (4 bytes length prefix) |
| **`zstring`** | String | len + 1 bytes | Null-terminated string (C-style string) |
| **`z16string`**| String | 2 * len + 2 bytes | Null-word-terminated UTF-16 style string |
| **`pad`** | None | `buf_len` bytes | Zero-filled padding bytes. Source value is ignored |
| **`ignore`** / **`-`** | Any | 0 bytes | The field is completely ignored during serialization |
| **`any`** | Any | Natural | Uses the Go field's natural primitive type encoding |
| **`custom`** | Any | Custom | Indicates custom serializer override (must be paired with `serializer`) |

---

## 3. Tag Options

Tag options are appended using comma separators:

### `encoding=NAME`
Configures the text encoding for string conversion.
* **Usage**: `binary:"string(10),encoding=shift-jis"`
* Supported encodings include `utf-8`, `shift-jis`, `euc-jp`, `utf-16le`, etc. (registered via `Marshaller.AddTextEncoding`).

### `endian=big|little|inverse`
Overrides the default byte order (endianness) for the field.
* **`big`**: Forces Big Endian.
* **`little`**: Forces Little Endian.
* **`inverse`**: Inverts the parent struct's configured byte order.
* **Usage**: `Value uint32 `binary:"uint32,endian=inverse"`` (propagates recursively to nested struct fields).

### `serializer=NAME`
Uses a custom registered `Serializer` to marshal and unmarshal this field.
* **Usage**: `Data MyCustomType `binary:"custom,serializer=MyCustomSerializer"``

### `omittable[=Expr]`
Marks a trailing field as optional.
* If `omittable` is set without expressions, reading will silently stop (without error) if `io.EOF` is encountered at the beginning of the field.
* If an expression is given (e.g. `omittable=LimitExpr`), serialization and deserialization will skip this field if the current byte index `n` is greater than or equal to the evaluated value.
* **Usage**: `Extra uint32 `binary:"uint32,omittable"``

### `range=min..max`
Enforces range constraints on integer, unsigned integer, and float fields during deserialization.
* **Usage**: `Value uint16 `binary:"uint16,range=1..100"``
* Boundaries can be left open:
  * `range=0..` (values $\ge$ 0).
  * `range=..100` (values $\le$ 100).
* If a value is out of range, the decoding fails with `ErrValidationError` wrapped inside a `DecodeError`.

### `match=pattern`
Enforces regular expression matching on string fields during deserialization.
* **Usage**: `Code string `binary:"string(4),match=^[A-Z]+$"``
* The regex pattern is precompiled once during struct analysis for optimal performance.
* If a string does not match the pattern, the decoding fails with `ErrValidationError` wrapped inside a `DecodeError`.

### `valueof=Expr` (encode-only)
Auto-computes this integer field's serialized value from other fields when marshalling, removing manual length/count bookkeeping. The field's own Go value is ignored on encode and is **not** modified (emit-only). Supports the `bytelen()` and `count()` functions. See [Computed Field Values](#8-computed-field-values-valueof).
* **Usage**: `NameLen uint16 `binary:"uint16,valueof=bytelen(Name)"``

---

## 4. Array and Buffer Size Notation

Both the array length and the string/padding buffer size are **expressions** (see [§5 Expressions](#5-expressions)), not just literal constants.

### Array Length Prefix: `[len]TYPE`
Specifies that a field is an array whose length is given by the expression `len`.
* **Usage**: `Data []int `binary:"[10]int16"``
* If a fixed-size Go array (e.g. `[4]string`) is used, the tag's array length can be omitted: `binary:"[]string(10)"`.

### String Buffer Size Postfix: `TYPE(buf_len)`
Limits or pads the string buffer to exactly `buf_len` bytes, where `buf_len` is an expression.
* **Usage**: `Name string `binary:"string(16)"`` (if shorter than 16 bytes, it will be zero-padded; if longer, it will be truncated).

---

## 5. Expressions

Wherever a tag takes a size or a computed value — array length `[len]`, string/padding buffer size `(buf_len)`, `omittable=Expr`, and `valueof=Expr` — it accepts an **expression**, not only a literal.

### Operands
* **Integer literals** in decimal, hex (`0x1F`), octal (`0o17`), or binary (`0b1010`); `_` digit separators are allowed (e.g. `1_024`).
* **Field references** — the name of another field in the same struct, evaluated from its current value.

### Operators
`+`, `-`, `*`, `/`, and parentheses `()`.

### Field-reference scope
* **Decode-side expressions** (`[len]`, `(buf_len)`) and `omittable` may reference only fields defined **before** the target field, because they are evaluated as the stream is read in order.
* **`valueof`** (encode-only) may reference **any** field, since the whole value is available when encoding.

### Examples
```go
type Packet struct {
	HeaderLength int    `binary:"uint8"`
	PayloadSize  int    `binary:"uint16"`

	// Length computed from other fields and a constant
	Payload      []byte `binary:"[PayloadSize - (HeaderLength * 2)]byte"`
	Tail         []byte `binary:"[0x10]byte"` // fixed 16-byte field via a hex constant
}
```

> Within `valueof` only, expressions may additionally call the built-in functions `bytelen(F)` and `count(F)` (see [§8](#8-computed-field-values-valueof)). These functions are **not** available in decode-side `[len]` / `(buf_len)` / `omittable` expressions.

---

## 6. Interface & Polymorphic Handling

`binarystruct` can serialize and deserialize fields of interface types (e.g., `interface{}` / `any`) using two distinct strategies:

### Strategy 1: Pre-assigned Interfaces (Static Type Resolution)
If a struct field is of an interface type, the decoder checks if the field has been **pre-assigned** with a concrete value before `Unmarshal` is called. If pre-assigned, the decoder automatically resolves the underlying concrete type:

```go
type Packet struct {
	Payload interface{} `binary:"any"` // resolves to the pre-assigned type's layout
}

// Pre-assign the interface with the concrete structure expected in the stream
var data int32 = 0
pkt := Packet{Payload: &data}

// Unmarshal decodes binary bytes directly into the 'data' variable
_, err := binarystruct.Unmarshal(blob, binarystruct.LittleEndian, &pkt)
```

### Strategy 2: Dynamic Allocation using a Custom Serializer
For network packets or files containing polymorphic payloads (e.g. Type-Length-Value or packet headers followed by dynamic bodies), you can use a custom `Serializer`. 

The custom deserializer can inspect previously decoded fields of the parent struct (e.g., a `Type` or `MessageID` field) and dynamically allocate the appropriate concrete type at runtime:

```go
type Packet struct {
	MsgType uint8       `binary:"uint8"`
	Payload interface{} `binary:"custom,serializer=DynamicPayload"`
}

func (s *DynamicPayloadSerializer) Deserialize(r io.Reader, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (value interface{}, n int, err error) {
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
	n, err = binarystruct.Read(r, order, payload)
	return payload, n, err
}
```

For a complete, compile-checked demonstration, see [example_interface_test.go](example_interface_test.go).

---

## 7. Optional & Omittable Fields

`binarystruct` allows fields at the end of a struct to be omitted during serialization and deserialization using the `omittable` option. This is particularly useful for backward-compatible binary format versions or variable-length payloads.

### EOF-based Omission
By applying the `omittable` option without any expression, you designate the field as optional based on the end of the input stream.
* **Unmarshal**: If the input stream is exhausted (`io.EOF` or `io.ErrUnexpectedEOF`) before reading any bytes for this field, the decoding completes successfully, and the field is left with its default value (or `nil` for pointers). If a partial read occurs, it still returns an error.
* **Marshal**: If the field is a pointer or interface type and is `nil`, it will be completely omitted from the output stream.

```go
type Packet struct {
	Required uint16
	Optional uint32 `binary:"uint32,omittable"`
}
```

### Expression-based Omission
By assigning an expression (e.g. `omittable=Expression`), the field is omitted dynamically depending on the current read/write offset.
* **Marshal & Unmarshal**: Before processing the field, the expression is evaluated. If the current byte count processed for the struct (`n`) is greater than or equal to the evaluated value, this field (and all subsequent fields) are skipped.
* This is typically used in structures where a header field defines the total message size.

```go
type Packet struct {
	TotalSize uint16
	Val1      uint32
	Extra1    uint32  `binary:"uint32,omittable=TotalSize"`
	Extra2    *uint32 `binary:"uint32,omittable=TotalSize"`
}
```

---

## 8. Computed Field Values: `valueof`

The `valueof` option auto-computes an integer field's serialized value from other fields during **encoding only**. It pairs naturally with a decode-side size expression so that a length field and the field it sizes stay in sync without manual bookkeeping:

```go
type Record struct {
	NameLen uint16 `binary:"uint16,valueof=bytelen(Name)"` // encode: set to len(Name)
	Name    []byte `binary:"[NameLen]byte"`                 // decode: sized from NameLen
}
```

### Functions
A `valueof` value is a full [expression](#5-expressions) (operators, constants, field references) **extended** with two built-in functions — available only inside `valueof` — each taking a single field-name argument:

| Function | Result |
| :--- | :--- |
| **`bytelen(F)`** | Total encoded byte length of field `F` (honors text encodings, length prefixes, arrays, and nested structs). Valid for any field. |
| **`count(F)`** | Element count (`len(F)`) of an **array or slice** field. Not valid for strings — use `bytelen` for a string's byte length. |

Examples: `valueof=bytelen(Name)`, `valueof=bytelen(Payload)+2`, `valueof=count(Items)`, `valueof=bytelen(A)+bytelen(B)`.

### Rules
* **Encode-only.** On decode the field is read normally; pair it with a decode-side size expression (e.g. `[NameLen]byte`) to size the target field.
* **Emit-only (no write-back).** The computed value is written to the stream; your struct field is left untouched. To populate the computed values in Go, perform a `Marshal`/`Unmarshal` (or `Write`/`Read`) round trip.
* **Integer/bitmap fields only.** Using `valueof` on any other field type is a compile-time error.
* **Forward references allowed.** Unlike decode-side size expressions (which may reference only *preceding* fields), a `valueof` expression may reference any field, since the entire value is available at encode time.
* `bytelen`/`count` are valid only inside `valueof`, not in `[len]` / `(buf_len)` size expressions.
* **Scope.** `valueof` derives only lengths and counts of fields (via `bytelen`/`count`). Other derived values — CRC checksums, compressed sizes, offsets, etc. — are not computed for you and must be assigned normally.

> **Codegen note:** the static code generator supports `count(F)`, and `bytelen(F)` of byte slices/arrays and non-encoded strings. `bytelen` of nested structs or text-encoded strings is not yet supported by codegen; use the runtime interpreter for those structs.
