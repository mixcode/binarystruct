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

| Tag Type | Go Kind | Encoded Size | Description |
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
| **`custom`** | Any | Custom | Indicates custom codec override (must be paired with `codec`) |

---

## 3. Tag Options

Tag options are appended using comma separators:

### `encoding=NAME`
Configures the text encoding for string conversion.
* **Usage**: `binary:"string(10),encoding=shift-jis"`
* Supported encodings include `utf-8`, `shift-jis`, `euc-jp`, `utf-16le`, etc. (registered via `Marshaler.AddTextEncoding`).

### Struct-level options: the `_ struct{}` sentinel
Declare struct-wide options once with a **blank `_ struct{}` sentinel field**. It
encodes to **zero bytes** (it is metadata, not a field). The type must be `struct{}`
— a `_` field of any other type carrying these options is rejected (it would
otherwise be encoded as data and the option silently dropped). Two options are
supported:

* **`endian=big|little`** — the struct's byte order, so `Marshal`/`Unmarshal`/… need
  no order argument.
* **`encoding=NAME`** — a default text encoding for the struct's string fields. A
  string field's own `encoding=` overrides it; with neither, the field falls back to
  the `Marshaler`'s `DefaultTextEncoding`. (The encoding must still be registered via
  `Marshaler.AddTextEncoding`.)

```go
type Header struct {
	_     struct{} `binary:"endian=big,encoding=shift-jis"` // order + default text encoding
	Magic uint32   `binary:"uint32"`
	Name  string   `binary:"wstring"`                       // encoded as shift-jis
}
```
* **Embedding** a struct that declares these propagates them (a reusable base, e.g.
  `type bigEndian struct{ _ struct{} \`binary:"endian=big"\` }`). Conflicting values
  from multiple embedded structs are an error.
* A value that declares no order (a bare scalar, a third-party struct) takes a
  fallback from `binarystruct.NewMarshalerOrder(order)`; otherwise encoding a
  multi-byte value fails with a clear error.
* **Codegen:** the static generator supports both struct-level `endian=` and
  struct-level `encoding=` (it bakes the encoding into each un-tagged string field,
  as the runtime does). Order/encoding *inheritance via embedding* is not supported
  by codegen — declare them directly on the struct there.

### `endian=big|little|inverse`
Per-field **override** of the struct's declared byte order.
* **`big`**: Forces Big Endian.
* **`little`**: Forces Little Endian.
* **`inverse`**: Inverts the inherited byte order.
* **Usage**: `Value uint32 `binary:"uint32,endian=inverse"`` (propagates recursively to nested struct fields).
* **This tag is an override only.** The struct declares its overall order (the `_` sentinel above); add per-field `endian=` **only to the fields that differ** (e.g. a mixed-endian format) — do **not** tag every field.

### `codec=NAME`
Uses a custom registered `Codec` to marshal and unmarshal this field.
* **Usage**: `Data MyCustomType `binary:"custom,codec=MyCustomCodec"``

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

### `const=Value` (encode + decode)
Pins a field to a fixed value — emitted on encode (the Go field is ignored) and validated on decode (`ErrValidationError` on mismatch). Ideal for magic numbers and signatures. Integer targets take an integer expression and are **endian-sensitive**; byte-sequence targets (`[N]byte`/`string(N)`) take a natural-order hex blob. See [Fixed / Magic Values](#9-fixed--magic-values-const).
* **Usage**: `Sig uint32 `binary:"uint32,const=0x04034b50,endian=little"`` or `Magic [8]byte `binary:"[8]byte,const=0x89504e470d0a1a0a"``

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
_, err := binarystruct.NewMarshalerOrder(binarystruct.LittleEndian).Unmarshal(blob, &pkt)
```

### Strategy 2: Dynamic Allocation using a Custom Codec
For network packets or files containing polymorphic payloads (e.g. Type-Length-Value or packet headers followed by dynamic bodies), you can use a custom `Codec`. 

The custom decodec can inspect previously decoded fields of the parent struct (e.g., a `Type` or `MessageID` field) and dynamically allocate the appropriate concrete type at runtime:

```go
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
* **Scope of the built-ins.** `bytelen`/`count` derive only lengths and counts of fields. Other derived values — CRC checksums, compressed sizes, etc. — are computed by **custom evaluators** you register (see below).

### Custom valueof evaluators (checksums, CRCs)

Register a named evaluator on the Marshaler and reference it from a `valueof=NAME(field, …)` tag to derive any value — the common case being a checksum over preceding fields:

```go
ms := binarystruct.NewMarshaler()
ms.AddValueOf("CRC32", func(c binarystruct.ValueOfContext) (uint64, error) {
    h := crc32.NewIEEE()
    for _, a := range c.Args { h.Write(a.Bytes) } // hash the ENCODED bytes
    return uint64(h.Sum32()), nil
})

type Chunk struct {
    _      struct{} `binary:"endian=big"`
    Length uint32   `binary:"uint32,valueof=bytelen(Data)"`
    Type   string   `binary:"string(4)"`
    Data   []byte   `binary:"[Length]byte"`
    CRC    uint32   `binary:"uint32,valueof=CRC32(Type, Data)"`
}
blob, _ := ms.Marshal(&Chunk{Type: "IHDR", Data: payload})
```

* **Per-Marshaler registration** (like custom `Codec`s): use `ms.Marshal`/`ms.Unmarshal`, not the package-level functions. `AddValueOf`/`RemoveValueOf`/`GetValueOf` manage the registry; an unregistered name fails loud. The name must not be `bytelen`/`count`.
* **Hash the encoded bytes**, not the Go values: each `ValueOfContext.Args[i]` carries `Bytes` (what is written to / read from the stream, honoring byte order and text encoding) and `Value` (the Go field value). A checksum must use `Bytes`.
* **Validated on decode.** Unlike the encode-only built-ins, a custom evaluator also runs on decode (over the decoded fields) and the result is compared to the value read; a mismatch is a `DecodeError` wrapping `ErrValidationError`. Validation is a post-decode pass, so a checksum may reference fields declared after it.
* **Must be the whole expression** — `valueof=CRC32(Type, Data)`; it cannot be mixed with arithmetic in this version.

> **Codegen note:** the static code generator resolves `count(F)` and `bytelen(F)` for nearly every field shape (scalars and scalar arrays, byte slices/arrays, all string variants including text-encoded, nested structs, tag-counted arrays of structs, and pointer-to-struct). It also supports **custom valueof evaluators**: byte-region fields (`[]byte`/`[N]byte`, raw `string`, constant-size `string(N)` without text encoding) and fixed-width integer scalars are emitted inline, and every other argument shape (text-encoded/prefixed strings, floats, multibyte-scalar arrays, padded byte slices, variable string buffers) is re-encoded with its own tag via `ms.MarshalAs` so the bytes match the runtime exactly. The one unsupported shape is a **nested-struct argument** — generation fails there (use the runtime). Generated code needs a non-nil Marshaler (call `WriteBinaryWithMarshaler`). Decode-time validation is **on by default** (generated decode recomputes and verifies the value, matching the runtime interpreter); the `-no-validate` flag strips all decode validation — this custom check plus the built-in `const`/`range`/`match` checks.

---

## 9. Fixed / Magic Values: `const`

The `const` option pins a field to a fixed value. It is **emitted on encode** (the Go field's value is ignored, like `valueof`) and **validated on decode** (a mismatch returns a `DecodeError` wrapping `ErrValidationError`). This is the natural way to express format signatures, version markers, and reserved fields without hand-writing "set this constant / check this constant" code.

```go
type ZIPLocalHeader struct {
	Signature uint32 `binary:"uint32,const=0x04034b50,endian=little"` // 'PK\x03\x04'
	Version   uint16 `binary:"uint16,const=20,endian=little"`
	// ... rest of header
}

type PNGHeader struct {
	Magic [8]byte `binary:"[8]byte,const=0x89504e470d0a1a0a"` // \x89PNG\r\n\x1a\n
}
```

### Two target shapes

| Target | Const syntax | Encoding |
| :--- | :--- | :--- |
| **Integer / bitmap** (`int8`…`uint64`, `byte`/`word`/`dword`/`qword`) | A constant integer **expression** — decimal, hex `0x1F`, octal `0o17`, binary `0b1010`, `+ - * /`, parens. | Written as an integer, **honoring the field's byte order** (see the endianness note). Must fit a signed 64-bit int (`< 2^63`). |
| **Byte sequence** (`[N]byte`, `[]byte`, `string(N)`) | A **hex blob** `0xAABBCC…` — the bytes in natural order; each byte is two hex digits, `_` separators allowed. | Written **verbatim** in natural order — endianness-independent. The field's fixed size must equal the constant's byte length. |

### Endianness note (integer consts)

An integer `const` is serialized through the active byte order, so `const=0x04034b50` emits `50 4b 03 04` under little-endian but `04 03 4b 50` under big-endian. **Add an explicit `endian=little|big` to the field** to make the signature's bytes deterministic regardless of the marshaller's order. A byte-sequence `const` is written in natural order and needs no `endian=` — indeed `PK\x03\x04` as a byte blob is simply `const=0x504b0304`, which reads more clearly than the byte-swapped integer `0x04034b50`.

### Rules
* **Emit-only on encode.** The Go field is not modified; whatever you leave in it is ignored and the constant is written. On decode the field is read from the stream and then validated against the constant.
* **Target types.** Integer/bitmap or a raw byte sequence only; any other type is a compile-time error. The byte form cannot be combined with `encoding=` (raw bytes only) and requires a fixed size (`[N]byte` or `string(N)`).
* **Not combinable with `valueof`** (the two both override the field's emitted value).
* **Codegen.** Both shapes are supported by the static code generator.
