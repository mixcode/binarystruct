[![Go Reference](https://pkg.go.dev/badge/github.com/mixcode/binarystruct.svg)](https://pkg.go.dev/github.com/mixcode/binarystruct) [![LLM Friendly](https://img.shields.io/badge/LLM-Friendly-blue)](llms.txt)

> [!NOTE]
> **AI Agents**: Read [llms-full.txt](llms-full.txt) for a complete system prompt manual, rules, and syntax details for code generation.

# binarystruct : binary data encoder/decoder for native Go structs

Package binarystruct is an automatic type-converting binary data encoder/decoder(or marshaller/unmarshaller) for go-language structs.

Go's built-in binary encoding package, "encoding/binary" is the preferred method to deal with binary data structures. The binary package is quite easy to use, but some cases require additional type conversions when values are tightly packed.
For example, an integer value in raw binary structure may be stored as a word or a byte, but the decoded value would be type-casted to an architecture-dependent integer for easy of use in the Go context.

This package simplifies the typecasting burdens by automatically handling conversion of struct fields using field tags.


## A Quick Example

Assume we have a binary data structure with a magic header and three integers, byte, word, dword each, like below.
By writing binary data types to field tags in Go struct definition, the values are automatically recognized and converted to proper encoding types.

```
// source binary data
blob := []byte { 0x61, 0x62, 0x63, 0x64,
	0x01,
	0x00, 0x02,
	0x00, 0x00, 0x00, 0x03 }
// [ "abcd", 0x01, 0x0002, 0x00000003 ]

// Go struct, with field types specified in tags. The blank `_` field declares
// the format's byte order, so Marshal/Unmarshal need no order argument.
strc := struct {
	_            struct{} `binary:"endian=big"` // this struct is big-endian
	Header       string   `binary:"[4]byte"`    // maps to 4 bytes
	ValueInt8    int      `binary:"int8"`        // maps to single signed byte
	ValueUint16  int      `binary:"uint16"`      // maps to two bytes
	ValueDword32 int      `binary:"dword"`       // maps to four bytes
}{}

// Unmarshal binary data into the struct
readsz, err := binarystruct.Unmarshal(blob, &strc)

// the structure have proper values now
fmt.Println(strc)
// {{} abcd 1 2 3}

// Marshal a struct to []byte
output, err := binarystruct.Marshal(&strc)
// output == blob

```

> **Byte order.** Declare a struct's byte order once with a blank `_ struct{}` field tagged `binary:"endian=big|little"` (or embed a struct that declares one); then `Marshal`/`Unmarshal`/`Write`/`Read`/`Append`/`Inspect` take **no** order argument. A per-field `endian=` tag overrides it for that field. For a value that declares no order (a bare scalar, a third-party struct), supply a fallback with `binarystruct.NewMarshalerOrder(order)`.

## Features

* **Automatic & Safe Type Conversions**: Effortlessly maps packed binary layouts into Go native types (e.g. converting `uint16` or `int8` streams directly into Go `int` fields) with range and bounds checks.
* **Declarative Validation**: Validate deserialized values inline with `range=min..max` for numeric bounds checks and `match=pattern` for regex string validation, returning errors on violation.
* **Fine-Grained Layout Controls**: Control data alignment using explicit types like `byte`, `word`, `dword`, `qword`, and zero-filled padding bytes via the `pad(size)` tag.
* **Dynamic Size Expressions**: Calculate array lengths and string buffer sizes dynamically based on other struct fields, supporting arithmetic operations (`+`, `-`, `*`, `/`) and parentheses (e.g., `[PayloadSize - (HeaderLength * 2)]byte`).
* **Computed Length/Count Fields**: Fill a length or count field automatically at encode time with `valueof=bytelen(F)` / `valueof=count(F)`, so you never hand-maintain a `NameLen` that must equal `len(Name)`. See [Computed Field Values](#computed-field-values-valueof).
* **Fixed / Magic Values**: Pin signatures and version fields with `const=` — emitted on encode and validated on decode (integer magics like `const=0x04034b50` or byte-sequence magics like `const=0x89504e470d0a1a0a`). See [Fixed / Magic Values](#fixed--magic-values-const).
* **Interface & Polymorphic Handling**: Automatically deserializes into pre-assigned interface fields, or uses custom codecs to dynamically allocate types based on previously decoded header values.
* **High-Performance Runtime Interpreter**: Uses dynamic layout compilation and a cached metadata interpreter. Unsafe Mode (default) bypasses reflection using `unsafe.Pointer` and zero-allocation slice streaming, yielding giant performance gain compared with safe mode using Go reflection.
* **Static Code Generation**: Includes a `binarystruct-codegen` tool that generates optimized, reflection-free `MarshalBinary` / `UnmarshalBinary` methods from struct tags. Achieves up to **6.7x speedup** over safe-mode reflection with near-zero allocations. Supports `go:generate` integration. See [`binarystruct-codegen/README.md`](binarystruct-codegen/README.md).
* **Multi-Language String Encoding**: Supports converting custom character encodings (e.g., `Shift-JIS`, `UTF-16`) on string fields by registering encodings via `AddTextEncoding` with customizable default fallback encodings.
* **Single-Value Marshalling**: Encode/deserialize standalone non-struct variables directly using `MarshalAs` / `UnmarshalAs` with custom tags.
* **Custom Codecs**: Register custom encoders/decoders via the `Codec` interface to handle complex validation or dynamic type mappings.
* **Struct Inspection Helper**: Includes an `Inspect` API that formats struct layouts, displaying field offsets, sizes, types, and values in customizable bases (decimal, hex, binary).
* **Safe Mode Fallback**: Pure reflection-based Go fallback activated via `-tags safe_binarystruct` build flag for restricted platforms like Google App Engine.

## Performance Modes (Safe vs. Unsafe / SIMD)

This package supports multiple build modes to balance performance, platform safety, and experimental hardware features:

| Mode / Build Tags | Description | Performance Profile |
| :--- | :--- | :--- |
| **Default Mode** (Unsafe) | Bypasses reflection using direct memory operations with `unsafe.Pointer` interpreter and layout-compatible fast-paths. | Faster than safe mode with fewer allocations (see [benchmark table](#performance-comparison)). |
| **Safe Mode** (`-tags safe_binarystruct`) | Falls back to pure reflection-based Go. Required on restricted platforms. | Standard Go reflection overhead. |
| **SIMD Mode** (`GOEXPERIMENT=simd -tags experiment_simd`) | Uses experimental Go 1.26 `simd/archsimd` to vectorize endian byte-swapping on AMD64 with CPU feature checks. | Maximum vectorized throughput for large arrays/slices. |

### Building for Restricted Platforms

If you deploy to sandboxed environments that restrict memory address access or block Go's `unsafe` package (e.g. Google App Engine standard environment), you must compile your project with the `safe_binarystruct` build tag:

```bash
go build -tags safe_binarystruct ./...
go test -tags safe_binarystruct ./...
```

---

## Computed Field Values (`valueof`)

Length and count fields usually have to mirror another field by hand — a filename-length field that must always equal `len(Name)`, for example. The `valueof` option computes such a field's serialized value at **encode time**, so you only maintain the data field:

```go
type Record struct {
	NameLen uint16 `binary:"uint16,valueof=bytelen(Name)"` // encode: written as the byte length of Name
	Name    []byte `binary:"[NameLen]byte"`                 // decode: sized from NameLen
}
```

A `valueof` value is a full [expression](STRUCT_TAGS.md#5-expressions) (arithmetic, constants, field references) extended with two functions:

* **`bytelen(F)`** — total encoded byte length of any field `F` (honors text encodings, length prefixes, arrays, and nested structs, not just `len()`).
* **`count(F)`** — element count of an array or slice field `F` (not valid for strings; use `bytelen` for a string's byte length).

`valueof` is **encode-only and emit-only**: the computed value is written to the stream, but your Go struct field is never modified. To read the computed values back into Go, do a `Marshal`/`Unmarshal` round trip. It derives only lengths and counts — other values such as CRC checksums, compressed sizes, or offsets are not computed for you. See the [Struct Tag Reference](STRUCT_TAGS.md#8-computed-field-values-valueof) for full details.

### Recipe: variable-length records

The most common real-world layout — a header carrying the byte-lengths (or element counts) of the variable data that follows — is a length field with `valueof=` paired with a `[len]` size expression on its target. Each pair stays in sync automatically: `valueof` fills the length on encode, the size expression consumes it on decode.

```go
type Record struct {
	Magic      uint32 `binary:"uint32"`                        // you set this
	NameLen    uint16 `binary:"uint16,valueof=bytelen(Name)"`  // auto = encoded length of Name
	PayloadLen uint32 `binary:"uint32,valueof=bytelen(Payload)"`
	ItemCount  uint16 `binary:"uint16,valueof=count(Items)"`   // auto = number of Items

	Name    []byte   `binary:"[NameLen]byte"`     // sized from NameLen on decode
	Payload []byte   `binary:"[PayloadLen]byte"`  // sized from PayloadLen
	Items   []uint32 `binary:"[ItemCount]uint32"` // sized from ItemCount
}

// Encode: set only the data fields; the length/count fields are computed.
rec := Record{Magic: 0x5A45, Name: []byte("file.txt"), Payload: data, Items: ids}
blob, _ := binarystruct.NewMarshalerOrder(binarystruct.LittleEndian).Marshal(&rec)
```

On encode you populate only `Name`, `Payload`, and `Items`; `NameLen`/`PayloadLen`/`ItemCount` are written from the actual data. On decode the size expressions read each field back at exactly the right length. (Because `valueof` is emit-only, `rec.NameLen` is still `0` in memory after `Marshal` — round-trip through `Unmarshal` if you need the struct populated.)

---

## Fixed / Magic Values (`const`)

The `const` option pins a field to a fixed value — **emitted on encode** (your Go field is ignored) and **validated on decode** (a mismatch returns an `ErrValidationError`). It is the natural way to express format signatures, version markers, and reserved fields:

```go
type ZIPLocalHeader struct {
	Signature uint32  `binary:"uint32,const=0x04034b50,endian=little"` // 'PK\x03\x04'
	Version   uint16  `binary:"uint16,const=20,endian=little"`
}

type PNGHeader struct {
	Magic [8]byte `binary:"[8]byte,const=0x89504e470d0a1a0a"` // \x89PNG\r\n\x1a\n
}
```

There are two target shapes:

* **Integer / bitmap** (`const=0x04034b50`): a constant integer expression. It is written through the byte order, so its bytes depend on endianness — **add an explicit `endian=little|big`** to make a signature deterministic. Limited to values below 2⁶³; use the byte-sequence form for larger or multi-byte magics.
* **Byte sequence** `[N]byte` / `string(N)` (`const=0x89504e470d0a1a0a`): a hex blob written in natural byte order, so it is **endianness-independent** — `PK\x03\x04` is simply `const=0x504b0304`. The field's fixed size must match the constant's length.

`const` cannot be combined with `valueof`, the byte form cannot use `encoding=`, and both shapes are supported by the static code generator. See the [Struct Tag Reference](STRUCT_TAGS.md#9-fixed--magic-values-const) for details.

---

## Struct Layout Inspection & Debugging

`binarystruct` includes an `Inspect` helper that analyzes a struct's binary layout and prints out the offset, size, and value of each field. This is extremely helpful for debugging byte alignment and padding issues.

```go
type Packet struct {
	Magic  string `binary:"[4]byte"`
	Length uint16 `binary:"uint16"`
	Flag   uint8  `binary:"uint8"`
	Data   []byte `binary:"[2]byte"`
}

pkt := Packet{Magic: "HEAD", Length: 12, Flag: 1, Data: []byte{0xaa, 0xbb}}

// Inspect the struct layout
layout, _ := binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Inspect(&pkt)

// Format and print it
format := binarystruct.DefaultLayoutFormat
format.BaseOffset = 16 // format offsets in hexadecimal
fmt.Println(layout.Format(format))
```

Output:
```text
+0x00(0x04) [4]byte Magic = [72 69 65 68] ("HEAD")
+0x04(0x02) uint16 Length = 12 (0x000c)
+0x06(0x01) uint8 Flag = 1 (0x01)
+0x07(0x02) [2]byte Data = [170 187]
```

> **Note**: If your structure contains custom codecs or encodings, use `ms.Inspect(&pkt)` on your custom-configured `Marshaler` instance (its byte order is set at construction via `NewMarshaler`) instead of the package-level `binarystruct.Inspect(&pkt, order)`, to ensure custom options are correctly recognized during inspection.

### Exporting Layout to JSON

You can export the analyzed layout metadata as a JSON schema. This is useful for integrating with external systems or generating schema structures in other languages:

```go
js, _ := layout.ToJSON()
fmt.Println(string(js))
```

---

## Static Code Generation for Production

`binarystruct` includes a standalone code generation tool that compiles struct layouts into static Go methods. In production, this completely eliminates runtime layout interpretation and reflection overhead, yielding maximum performance.

### Installation
Install the code generator CLI:
```bash
go install github.com/mixcode/binarystruct/binarystruct-codegen@latest
```

### Usage
Generate static `MarshalBinary` and `UnmarshalBinary` methods for your structs:
```bash
binarystruct-codegen -type MyStruct,MyNestedStruct [path/to/package/directory]
```
By default, it writes the generated code to `<first_type>_binary.go` in the same directory.

### Go Generate Integration
We recommend integrating it into your Go source files using `go generate`:
```go
//go:generate binarystruct-codegen -type Packet,Header
type Packet struct {
	Magic uint32 `binary:"uint32"`
	Data  []byte `binary:"[10]byte"`
}
```
Run `go generate ./...` to compile your serialization methods.

### How It Works
* The generated code implements standard Go `encoding.BinaryMarshaler` and `encoding.BinaryUnmarshaler` interfaces, and high-performance streaming interfaces (`BinaryReader` / `BinaryWriter`).
* If custom codecs or text encodings are present, context-aware interfaces (`MarshalerContextReader` / `MarshalerContextWriter`) are generated to automatically retrieve custom handlers from the `Marshaler` context at runtime.
* The main `binarystruct.Marshal` and `binarystruct.Unmarshal` library calls automatically detect these generated methods and fast-path to executing them directly.

### Performance Comparison

A deserialization benchmark decoding a realistic 280-byte packet containing range validations, a nested struct with 8 fields, and a dynamic slice of bytes yielded the following results (on a 13th Gen Intel Core i5-13600K):

| Mode / Strategy | Execution Time | Heap Allocations | Performance Boost |
| :--- | :--- | :--- | :--- |
| **Safe Mode** (`-tags safe_binarystruct`) | `4,260 ns/op` | `47 allocs/op` | Baseline |
| **Unsafe Mode** (Default Interpreter) | `3,670 ns/op` | `22 allocs/op` | +16% Speed, -53% Allocations |
| **Static Codegen** (Compiled) | `634 ns/op` | `8 allocs/op` | **+570% Speed (6.7x faster)**, **-83% Allocations** |

---

## Detailed Error Reporting with Byte Offset

When unmarshalling binary payloads, failures (such as premature EOF) return errors wrapped in a custom `DecodeError` struct. This allows you to inspect the exact byte offset and field name where the failure occurred:

```go
_, err := binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Unmarshal(corruptedData, &pkt)
if err != nil {
	var decodeErr *binarystruct.DecodeError
	if errors.As(err, &decodeErr) {
		fmt.Printf("Error at byte offset %d, field %q: %v\n", 
			decodeErr.Offset, decodeErr.Field, decodeErr.Err)
	}
}
```

---

## See also
* [Struct Tag Reference Manual](STRUCT_TAGS.md) for details about tag types, options, and dynamic math expressions.
* [Go Reference Doc](https://pkg.go.dev/github.com/mixcode/binarystruct) for Go package documentation.
