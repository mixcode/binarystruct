# binarystruct-codegen

Static code generator for [binarystruct](https://github.com/mixcode/binarystruct). Generates optimized `MarshalBinary` / `UnmarshalBinary` methods from struct `binary:"..."` tags, eliminating runtime reflection overhead.

## Install

```bash
go install github.com/mixcode/binarystruct/binarystruct-codegen@latest
```

## Usage

```
binarystruct-codegen -type TypeName[,TypeName2,...] [flags] [directory]
```

### Flags

| Flag | Description |
| :--- | :--- |
| `-type` | Comma-separated list of struct type names to generate methods for (**required**). |
| `-endian` | Fallback byte order (`big` or `little`) baked into the no-arg `MarshalBinary`/`UnmarshalBinary`/`AppendBinary` methods. **Optional** when the struct declares its own order (a blank `_ struct{}` field tagged `binary:"endian=…"`, which wins); otherwise required. Generation **errors** if neither the struct nor this flag gives an order. Not needed with `-json`. |
| `-output` | Output file name (default: `<first_type>_binary.go` or `<first_type>.json` if `-json` is set). |
| `-json` | Export parsed struct layout metadata to JSON instead of generating Go code. |
| `-tests` | Include test files (`*_test.go`) when parsing package files. |

### Arguments

| Argument | Description |
| :--- | :--- |
| `[directory]` | Go package directory containing struct definitions (default: `.`). |

## Examples

```bash
# Generate methods for Packet and Header types (big-endian) in the current directory
binarystruct-codegen -type Packet,Header -endian big

# Specify output file and source directory
binarystruct-codegen -type Packet -endian little -output packet_gen.go ./protocol

# Export structural metadata for Packet as a JSON schema layout (no -endian needed)
binarystruct-codegen -type Packet -json -output layout.json
```

### With `go:generate`

```go
//go:generate binarystruct-codegen -type Packet,Header -endian big
```

## What Gets Generated

For each specified type, the tool generates (the no-arg methods bake the `-endian` order):

- `MarshalBinary() ([]byte, error)` — implements `encoding.BinaryMarshaler`
- `AppendBinary(b []byte) ([]byte, error)` — implements `encoding.BinaryAppender` (Go 1.24)
- `UnmarshalBinary(data []byte) error` — implements `encoding.BinaryUnmarshaler`
- `WriteBinary(w io.Writer, order ByteOrder) (int, error)` / `ReadBinary(r io.Reader, order ByteOrder) (int, error)` — order-taking forms binarystruct dispatches to

If the struct uses features requiring a `Marshaler` context (text encodings via `encoding=`, custom codecs via `codec=`), context-aware methods are also generated:

- `WriteBinaryWithMarshaler(ms *Marshaler, w io.Writer, order ByteOrder) (int, error)`
- `ReadBinaryWithMarshaler(ms *Marshaler, r io.Reader, order ByteOrder) (int, error)`

These implement `MarshalerContextWriter` / `MarshalerContextReader`, enabling the binarystruct runtime to dispatch directly to the generated code when called through a `Marshaler`.

## Supported Tag Features

The binarystruct-codegen tool supports the full `binary:"..."` tag syntax including:

- All primitive types (`int8`–`int64`, `uint8`–`uint64`, `float32`, `float64`, `byte`, `word`, `dword`, `qword`)
- String types (`string(N)`, `bstring`, `wstring`, `dwstring`, `zstring`, `z16string`)
- Arrays (`[N]type`, `[Expr]type`)
- Padding (`pad(N)`)
- Tag math expressions (e.g. `string(PayloadSize - 4)`)
- Validation (`range=min..max`, `match=pattern`)
- Omittable fields (`omittable`)
- Struct-level byte order (`binary:"endian=big|little"`) and struct-level default text encoding (`binary:"encoding=NAME"`) via the blank `_ struct{}` sentinel
- Per-field endian override (`endian=big|little`)
- Text encoding (`encoding=NAME`)
- Custom codecs (`custom,codec=NAME`)
- Nested structs

**Not supported by codegen** (generation errors with a clear message — use the
runtime interpreter): struct-level `endian=inverse`, byte-order/encoding
inheritance via embedding, a self-referential `valueof=bytelen(F)` where `F`
is `string(thatVeryField)`, and **custom `valueof` evaluators** (registered on a
Marshaler at run time, so they cannot be embedded in standalone code). Per-field
`endian=inverse` and per-field `encoding=` are supported.

For the complete tag reference, see [STRUCT_TAGS.md](../STRUCT_TAGS.md) in the parent project.

## Runnable Example

A fully self-contained runnable example is provided in the [example](example) directory:
- [types.go](example/types.go) — Defines a `Packet` struct with tag declarations and a `go:generate` directive.
- [example_test.go](example/example_test.go) — Demonstrates marshaling and unmarshaling using the generated methods.

## Documentation

For full library documentation, tag syntax details, and benchmarks, see the [binarystruct README](https://github.com/mixcode/binarystruct).
