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

---

## 4. Array and Buffer Size Notation

### Array Length Prefix: `[len]TYPE`
Specifies that a field is an array of size `len`.
* **Usage**: `Data []int `binary:"[10]int16"``
* If a fixed-size Go array (e.g. `[4]string`) is used, the tag's array length can be omitted: `binary:"[]string(10)"`.

### String Buffer Size Postfix: `TYPE(buf_len)`
Limits or pads the string buffer to exactly `buf_len` bytes.
* **Usage**: `Name string `binary:"string(16)"`` (if shorter than 16 bytes, it will be zero-padded; if longer, it will be truncated).

---

## 5. Dynamic Expressions

Both array length `[len]` and string buffer size `(buf_len)` can use **dynamic expressions** referencing other struct fields instead of literal constants.

* **Supported Operators**: `+`, `-`, `*`, `/`, and parentheses `()`.
* **Field References**: Evaluated dynamically based on the current value of the referenced fields.

### Example
```go
type Packet struct {
	HeaderLength int    `binary:"uint8"`
	PayloadSize  int    `binary:"uint16"`
	
	// Buffer size is dynamically calculated using other fields
	Payload      []byte `binary:"[PayloadSize - (HeaderLength * 2)]byte"`
}
```
