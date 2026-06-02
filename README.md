[![Go Reference](https://pkg.go.dev/badge/github.com/mixcode/binarystruct.svg)](https://pkg.go.dev/github.com/mixcode/binarystruct)

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

// Go struct, with field types specified in tags
strc := struct {
	Header       string `binary:"[4]byte"` // maps to 4 bytes
	ValueInt8    int    `binary:"int8"`    // maps to single signed byte
	ValueUint16  int    `binary:"uint16"`  // maps to two bytes
	ValueDword32 int    `binary:"dword"`   // maps to four bytes
}{}

// Unmarshal binary data into the struct
readsz, err := binarystruct.Unmarshal(blob, binarystruct.BigEndian, &strc)

// the structure have proper values now
fmt.Println(strc)
// {abcd 1 2 3}

// Marshal a struct to []byte
output, err := binarystruct.Marshal(&strc, binarystruct.BigEndian)
// output == blob

```

## Features

* **Automatic & Safe Type Conversions**: Effortlessly maps packed binary layouts into Go native types (e.g. converting `uint16` or `int8` streams directly into Go `int` fields) with range and bounds checks.
* **Fine-Grained Layout Controls**: Control data alignment using explicit types like `byte`, `word`, `dword`, `qword`, and zero-filled padding bytes via the `pad(size)` tag.
* **Dynamic Size Expressions**: Calculate array lengths and string buffer sizes dynamically based on other struct fields, supporting arithmetic operations (`+`, `-`, `*`, `/`) and parentheses (e.g., `[PayloadSize - (HeaderLength * 2)]byte`).
* **High-Performance Structure Layout Interpreter**: Uses dynamic layout compilation and a cached metadata interpreter. Unsafe Mode (default) bypasses reflection using `unsafe.Pointer` and zero-allocation slice streaming, yielding up to **214x speedups** and **99.9% fewer allocations** than standard Go reflections.
* **Interface & Polymorphic Handling**: Automatically deserializes into pre-assigned interface fields, or uses custom serializers to dynamically allocate types based on previously decoded header values.
* **Multi-Language String Encoding**: Supports converting custom character encodings (e.g., `Shift-JIS`, `UTF-16`) on string fields by registering encodings via `AddTextEncoding` with customizable default fallback encodings.
* **Field-Level Endian Markings**: Override default byte orders per field (e.g., `endian=big`, `little`, or `inverse`), with recursive propagation down into nested structs.
* **Single-Value Marshalling**: Serialize/deserialize standalone non-struct variables directly using `MarshalAs` / `UnmarshalAs` with custom tags.
* **Custom Serializers**: Register custom encoders/decoders via the `Serializer` interface to handle complex validation or dynamic type mappings.
* **Struct Inspection Helper**: Includes an `Inspect` API that formats struct layouts, displaying field offsets, sizes, types, and values in customizable bases (decimal, hex, binary).
* **Safe Mode Fallback**: Pure reflection-based Go fallback activated via `-tags safe` build flag for restricted platforms like Google App Engine.

## Performance Modes (Safe vs. Unsafe / SIMD)

This package supports multiple build modes to balance performance, platform safety, and experimental hardware features:

| Mode / Build Tags | Description | Performance Profile |
| :--- | :--- | :--- |
| **Default Mode** (Unsafe) | Bypasses reflection using direct memory operations with `unsafe.Pointer` interpreter and layout-compatible fast-paths. | **Maximum Speed** (up to 214x faster, 99.9% fewer allocations). |
| **Safe Mode** (`-tags safe`) | Falls back to pure reflection-based Go. Required on restricted platforms. | Standard Go reflection overhead. |
| **SIMD Mode** (`GOEXPERIMENT=simd -tags experiment_simd`) | Uses experimental Go 1.26 `simd/archsimd` to vectorize endian byte-swapping on AMD64 with CPU feature checks. | Maximum vectorized throughput for large arrays/slices. |

### Building for Restricted Platforms

If you deploy to sandboxed environments that restrict memory address access or block Go's `unsafe` package (e.g. Google App Engine standard environment), you must compile your project with the `safe` build tag:

```bash
go build -tags safe ./...
go test -tags safe ./...
```

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
layout, _ := binarystruct.Inspect(&pkt, binarystruct.BigEndian)

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

---

## See also
* [Struct Tag Reference Manual](STRUCT_TAGS.md) for details about tag types, options, and dynamic math expressions.
* [Go Reference Doc](https://pkg.go.dev/github.com/mixcode/binarystruct) for API documentation.
