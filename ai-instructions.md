# AI and LLM Agent Instructions for binarystruct

This document contains rules, syntax constraints, and patterns to help LLM agents and code assistants successfully integrate and use the `binarystruct` library.

---

## 1. Syntax Cheat Sheet

`binarystruct` uses the `binary` struct tag to map Go struct fields to binary layouts.

```go
`binary:"[array_len]TYPE(buf_len),option1=val1,option2"`
```

### Supported Types
* **Fixed Scalars**: `int8`, `int16`, `int32`, `int64`, `uint8`, `uint16`, `uint32`, `uint64`
* **Bitmaps (type-agnostic)**: `byte` (1 byte), `word` (2 bytes), `dword` (4 bytes), `qword` (8 bytes)
* **Floats**: `float32`, `float64`
* **Strings**:
  * `string`: Raw byte string (padded with `0` up to `buf_len` if specified)
  * `bstring`, `wstring`, `dwstring`: Length-prefixed string (1, 2, or 4-byte length prefix)
  * `zstring`, `z16string`: Null-terminated string (C-style or UTF-16 style)
* **Padding**: `pad(size)` (inserts zero bytes on marshal; skips bytes on unmarshal)
* **Other**: `ignore` or `-` (skips field), `any` (default primitive layout), `custom` (custom serializer)

### Key Options
* `encoding=NAME`: String character conversion (e.g., `shift-jis`, `utf-16le`).
* `endian=big|little|inverse`: Byte order override. `inverse` flips the parent's byte order recursively.
* `serializer=NAME`: Reference to a custom registered serializer.
* `omittable[=Expression]`: Marks a trailing field as optional (suppresses `io.EOF` errors at start of field or skips based on struct byte-offset check).

---

## 2. Crucial Coding Rules & Constraints

### A. String Encodings Must Be Registered
* **Rule**: There are no default text encodings (other than raw UTF-8 bytes). Using `encoding=shift-jis` or similar in a tag will fail unless you first register the encoding on the `Marshaller` instance.
* **Code pattern**:
  ```go
  var ms binarystruct.Marshaller
  ms.AddTextEncoding("shift-jis", htmlindex.Get("shift-jis"))
  ```

### B. Interface & Polymorphic Payload Unmarshalling
When unmarshalling into an interface field, you must choose one of two patterns:
1. **Strategy 1 (Pre-assigned)**: Pre-initialize the interface field with a pointer to a concrete structure before calling `Unmarshal`.
   ```go
   var data MessageA
   pkt := Packet{Payload: &data}
   binarystruct.Unmarshal(blob, order, &pkt)
   ```
2. **Strategy 2 (Custom Serializer)**: Implement a custom `Serializer` that reads a preceding type field and dynamically allocates the concrete type.
   ```go
   func (s *DynamicPayloadSerializer) Deserialize(r io.Reader, parent reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (interface{}, int, error) {
       msgType := parent.FieldByName("MsgType").Uint()
       // Allocate dynamically and decode...
   }
   ```

### C. Build Tags for Restricted Environments
* **Rule**: If the code is compiled for platforms that restrict Go's `unsafe` package (e.g., Google App Engine), compile and test with the `safe_binarystruct` build tag:
  ```bash
  go test -tags safe_binarystruct ./...
  ```

### D. Dynamic Size Expressions
* Size expressions like `[PayloadSize - 2]byte` can only reference fields defined **before** the target field in the struct definition.

---

## 3. Debugging Layout Issues
If you encounter layout or padding issues, use the `Inspect` API to print out the struct's byte offsets, sizes, and values.

```go
layout, _ := binarystruct.Inspect(&myStruct, binarystruct.BigEndian)
fmt.Println(layout.Format(binarystruct.DefaultLayoutFormat))
```
