[![Go Reference](https://pkg.go.dev/badge/github.com/mixcode/binarystruct.svg)](https://pkg.go.dev/github.com/mixcode/binarystruct) [![LLM Friendly](https://img.shields.io/badge/LLM-Friendly-blue)](llms.txt)

> [!NOTE]
> **AI Agents**: Read [llms-full.txt](llms-full.txt) for a complete system prompt manual, rules, and syntax details for code generation.

> [!IMPORTANT]
> **Upgrading from 0.2.x?** v0.3.0 has breaking changes (the `Marshaler`/`Codec` renames, struct-declared byte order, an order-free API). See **[MIGRATION.md](MIGRATION.md)**.

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
* **Multidimensional Arrays**: Stack length prefixes (`[2][3]int16`, `[Rows][Cols]uint8`) to encode/decode nested Go arrays and slices in row-major order; each dimension is its own expression. See [Struct Tag Reference](STRUCT_TAGS.md#4-array-and-buffer-size-notation).
* **Computed & Derived Fields**: Fill a length or count field automatically at encode time with `valueof=bytelen(F)` / `valueof=count(F)`, so you never hand-maintain a `NameLen` that must equal `len(Name)`. For checksums/CRCs and other derived values, register a **custom evaluator** (`Marshaler.AddValueOf`) and reference it as `valueof=CRC32(Type, Data)` — computed on encode and validated on decode. See [Computed Field Values](#computed-field-values-valueof).
* **Fixed / Magic Values**: Pin signatures and version fields with `const=` — emitted on encode and validated on decode (integer magics like `const=0x04034b50` or byte-sequence magics like `const=0x89504e470d0a1a0a`). See [Fixed / Magic Values](#fixed--magic-values-const).
* **Interface & Polymorphic Handling**: Automatically deserializes into pre-assigned interface fields, or uses custom codecs to dynamically allocate types based on previously decoded header values.
* **High-Performance Runtime Interpreter**: Uses dynamic layout compilation and a cached metadata interpreter. Unsafe Mode (default) bypasses reflection using `unsafe.Pointer` and zero-allocation slice streaming, yielding giant performance gain compared with safe mode using Go reflection.
* **Static Code Generation**: Includes a `binarystruct-codegen` tool that generates optimized, reflection-free `MarshalBinary` / `UnmarshalBinary` methods from struct tags. Achieves **several-fold speedups** over the reflection interpreter with the fewest allocations of any mode. Supports `go:generate` integration. See [`binarystruct-codegen/README.md`](binarystruct-codegen/README.md).
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

Length and count fields usually have to mirror another field by hand. The `valueof` option computes such a field's serialized value at **encode time**, so you maintain only the data field. The built-ins are `bytelen(F)` (the encoded byte length of any field) and `count(F)` (an array/slice's element count); for derived values like checksums you can register a **custom evaluator** (`Marshaler.AddValueOf`, e.g. `valueof=CRC32(Type, Data)`) that also validates on decode. `valueof` is **emit-only** — it writes the stream but leaves your Go field unchanged (round-trip through `Unmarshal` to read it back). Full details in the [Struct Tag Reference](STRUCT_TAGS.md#8-computed-field-values-valueof).

### Recipe: variable-length records

The most common real-world layout — a header carrying the byte-lengths (or element counts) of the variable data that follows — pairs a `valueof=` length field with a `[len]` size expression on its target. Each pair stays in sync automatically: `valueof` fills the length on encode, the size expression consumes it on decode.

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

The `const` option pins a field to a fixed value — **emitted on encode** (your Go field is ignored) and **validated on decode** (mismatch → `ErrValidationError`) — ideal for format signatures and version markers:

```go
type PNGHeader struct {
	Magic [8]byte `binary:"[8]byte,const=0x89504e470d0a1a0a"` // \x89PNG\r\n\x1a\n
}
```

Integer magics (`const=0x04034b50`) are endian-sensitive — add `endian=` for a deterministic signature; byte-sequence magics on `[N]byte`/`string(N)` are written in natural order (endian-independent). Both are codegen-supported. See the [Struct Tag Reference](STRUCT_TAGS.md#9-fixed--magic-values-const).

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

(If the struct uses custom codecs/encodings, call `Inspect` on the same configured `Marshaler` you marshal with.) The layout also exports to a JSON schema via `layout.ToJSON()` — useful for tooling or generating types in other languages.

---

## Static Code Generation for Production

For maximum performance, the standalone **[`binarystruct-codegen`](binarystruct-codegen/README.md)** tool compiles your struct tags into static `MarshalBinary`/`UnmarshalBinary` methods, eliminating runtime reflection and layout interpretation. Install it, add a `//go:generate` directive, and `binarystruct.Marshal`/`Unmarshal` automatically fast-path to the generated code. See the **[code generator guide](binarystruct-codegen/README.md)** for installation, flags, supported features, and `go:generate` usage.

### Performance Comparison

The table below is produced by the committed cross-mode benchmark suite in [`bench/`](bench) — the same struct encoded/decoded by the **safe** runtime, the **unsafe** runtime (default), and the **static codegen** path — across four representative shapes: a flat scalar `Header`, a 1024-element `IntSlice`, a variable-length `Record`, and a `Nested` struct slice. Regenerate it for your hardware with `make bench`.

<!-- BENCH:START (generated by `make bench` — do not edit by hand) -->
| Workload | Op | Safe (runtime) | Unsafe (runtime) | Codegen | Codegen speedup |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Header** | Marshal | 402 ns / 4 allocs | 361 ns / 4 allocs | 228 ns / 3 allocs | 1.6× |
| **Header** | Unmarshal | 463 ns / 4 allocs | 410 ns / 4 allocs | 199 ns / 2 allocs | 2.1× |
| **IntSlice** | Marshal | 7,015 ns / 6 allocs | 5,906 ns / 6 allocs | 6,080 ns / 5 allocs | 1.0× |
| **IntSlice** | Unmarshal | 7,604 ns / 6 allocs | 6,864 ns / 6 allocs | 6,645 ns / 4 allocs | 1.0× |
| **Record** | Marshal | 635 ns / 6 allocs | 515 ns / 6 allocs | 378 ns / 5 allocs | 1.4× |
| **Record** | Unmarshal | 702 ns / 6 allocs | 569 ns / 6 allocs | 413 ns / 5 allocs | 1.4× |
| **Nested** | Marshal | 4,297 ns / 71 allocs | 3,956 ns / 71 allocs | 3,901 ns / 70 allocs | 1.0× |
| **Nested** | Unmarshal | 4,791 ns / 69 allocs | 4,278 ns / 69 allocs | 3,985 ns / 67 allocs | 1.1× |

> Measured with go1.26.3 on this machine via `make bench` (mean of the run). Numbers are hardware-dependent — **re-run `make bench` for your environment.** Lower is better; speedup = unsafe ÷ codegen.
<!-- BENCH:END -->

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
