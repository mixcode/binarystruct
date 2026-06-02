# codegen

Static code generator for [binarystruct](https://github.com/mixcode/binarystruct). Generates optimized `MarshalBinary` / `UnmarshalBinary` methods from struct `binary:"..."` tags, eliminating runtime reflection overhead.

## Install

```bash
go install github.com/mixcode/binarystruct/codegen@latest
```

## Usage

```
codegen -type TypeName[,TypeName2,...] [flags] [directory]
```

### Flags

| Flag | Description |
| :--- | :--- |
| `-type` | Comma-separated list of struct type names to generate methods for (**required**). |
| `-output` | Output file name (default: `<first_type>_binary.go`). |

### Arguments

| Argument | Description |
| :--- | :--- |
| `[directory]` | Go package directory containing struct definitions (default: `.`). |

## Examples

```bash
# Generate methods for Packet and Header types in the current directory
codegen -type Packet,Header

# Specify output file and source directory
codegen -type Packet -output packet_gen.go ./protocol
```

### With `go:generate`

```go
//go:generate codegen -type Packet,Header
```

## What Gets Generated

For each specified type, the tool generates:

- `MarshalBinary() ([]byte, error)` — implements `encoding.BinaryMarshaler`
- `UnmarshalBinary(data []byte) error` — implements `encoding.BinaryUnmarshaler`

If the struct uses features requiring a `Marshaller` context (text encodings via `encoding=`, custom serializers via `serializer=`), context-aware methods are also generated:

- `WriteBinaryWithMarshaller(ms *Marshaller, w io.Writer, order ByteOrder) (int, error)`
- `ReadBinaryWithMarshaller(ms *Marshaller, r io.Reader, order ByteOrder) (int, error)`

These implement `MarshallerContextWriter` / `MarshallerContextReader`, enabling the binarystruct runtime to dispatch directly to the generated code when called through a `Marshaller`.

## Supported Tag Features

The codegen tool supports the full `binary:"..."` tag syntax including:

- All primitive types (`int8`–`int64`, `uint8`–`uint64`, `float32`, `float64`, `byte`, `word`, `dword`, `qword`)
- String types (`string(N)`, `bstring`, `wstring`, `dwstring`, `zstring`, `z16string`)
- Arrays (`[N]type`, `[Expr]type`)
- Padding (`pad(N)`)
- Tag math expressions (e.g. `string(PayloadSize - 4)`)
- Validation (`range=min..max`, `match=pattern`)
- Omittable fields (`omittable`)
- Endian override (`endian=big|little|inverse`)
- Text encoding (`encoding=NAME`)
- Custom serializers (`custom,serializer=NAME`)
- Nested structs

For the complete tag reference, see [STRUCT_TAGS.md](../STRUCT_TAGS.md) in the parent project.

## Documentation

For full library documentation, tag syntax details, and benchmarks, see the [binarystruct README](https://github.com/mixcode/binarystruct).
