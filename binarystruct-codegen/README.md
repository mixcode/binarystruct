# binarystruct-codegen

Static code generator for [binarystruct](https://github.com/mixcode/binarystruct). Generates optimized `MarshalBinary` / `UnmarshalBinary` methods from struct `binary:"..."` tags, eliminating runtime reflection overhead.

## Install

```bash
go install github.com/mixcode/binarystruct/binarystruct-codegen@latest
```

This is a **separate nested module** (its own `go.mod`), so it is not reachable by
import path from a consumer module and is **not** covered by a `replace` directive
on the parent `binarystruct` module. To run an **unreleased / local checkout** (e.g.
a `replace`'d copy), build the tool directly from its directory instead of
`go install`:

```bash
# from a local clone / replaced checkout of binarystruct:
go build -o ./binarystruct-codegen ./binarystruct-codegen   # run from the repo root
# or build straight from the nested module directory:
cd path/to/binarystruct/binarystruct-codegen && go build -o /tmp/binarystruct-codegen .
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
| `-no-validate` | Strip **all** decode-time validation from the generated read methods — the `const`/`range`/`match` checks and custom-`valueof` recompute-and-compare. Default off: the generated decode validates everything, matching the runtime interpreter. Set this for trusted-input / hot-path decoding. |

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

Context-aware variants are **always** generated too (the no-arg forms above simply call them with a `nil` Marshaler):

- `WriteBinaryWithMarshaler(ms *Marshaler, w io.Writer, order ByteOrder) (int, error)`
- `ReadBinaryWithMarshaler(ms *Marshaler, r io.Reader, order ByteOrder) (int, error)`

These implement `MarshalerContextWriter` / `MarshalerContextReader`, enabling the binarystruct runtime to dispatch directly to the generated code when called through a `Marshaler`. You **must** call these (not the no-arg `MarshalBinary`/`UnmarshalBinary`) when the struct relies on a `Marshaler` context — text encodings via `encoding=`, custom codecs via `codec=`, or custom `valueof` evaluators via `valueof=NAME(...)` — since the no-arg forms pass a `nil` Marshaler and return a clear error for those fields.

## Supported Tag Features

The binarystruct-codegen tool supports the full `binary:"..."` tag syntax including:

- All primitive types (`int8`–`int64`, `uint8`–`uint64`, `float32`, `float64`, `byte`, `word`, `dword`, `qword`)
- String types (`string(N)`, `bstring`, `wstring`, `dwstring`, `zstring`, `z16string`)
- Arrays (`[N]type`, `[Expr]type`)
- Padding (`pad(N)`)
- Tag math expressions (e.g. `string(PayloadSize - 4)`)
- Validation (`range=min..max`, `match=pattern`, and `const=Value` magic/fixed values) — checked on decode by default; see `-no-validate`
- Computed field values (`valueof=bytelen(F)`, `valueof=count(F)` with arithmetic, plus custom evaluators — see below)
- Omittable fields (`omittable`)
- Struct-level byte order (`binary:"endian=big|little"`) and struct-level default text encoding (`binary:"encoding=NAME"`) via the blank `_ struct{}` sentinel
- Per-field endian override (`endian=big|little`)
- Text encoding (`encoding=NAME`)
- Custom codecs (`custom,codec=NAME`)
- Nested structs

**Custom `valueof` evaluators** (e.g. `valueof=CRC32(Type, Data)`, registered on a
Marshaler with `AddValueOf`) are supported for **any argument shape except a
nested struct**. Byte-region args (`[]byte`/`[N]byte`, raw `string`, constant-size
`string(N)` without text encoding) and fixed-width integer scalars
(`uint8`…`uint64`, signed, `byte`/`word`/`dword`/`qword`) are emitted inline; every
other shape (text-encoded or prefixed/terminated strings, floats, multibyte-scalar
arrays, padded byte slices, variable string buffers) is re-encoded with its own tag
via `ms.MarshalAs`, so the bytes match the runtime exactly. Like `codec=`, they
need a non-nil Marshaler, so use `WriteBinaryWithMarshaler` (the no-arg
`MarshalBinary` errors). Decode-time validation is **on by default** (generated
decode recomputes and verifies the value, matching the runtime interpreter); see
`-no-validate` below.

**Multidimensional arrays** (`[2][3]int16`, `[2][2][2]int8`) are supported for a
**scalar leaf type** — fixed Go arrays, and slices whose dimensions are all
specified (each slice level is allocated with `make` on decode). A non-scalar leaf
(string, nested struct, pointer) or mixed fixed-array/slice nesting falls back to
the runtime.

**Not supported by codegen** (generation errors with a clear message — use the
runtime interpreter): multidimensional arrays over a non-scalar leaf (per above),
struct-level `endian=inverse`, byte-order/encoding inheritance via embedding, a
self-referential `valueof=bytelen(F)` where `F` is `string(thatVeryField)`, and a
custom `valueof` evaluator referencing a **nested-struct** field. Per-field
`endian=inverse` and per-field `encoding=` are supported.

For the complete tag reference, see [STRUCT_TAGS.md](../STRUCT_TAGS.md) in the parent project.

## Runnable Example

A fully self-contained runnable example is provided in the [example](example) directory:
- [types.go](example/types.go) — Defines a `Packet` (struct-level `endian=`, a `const` magic, and `range=` validation) and a `Chunk` (a PNG-chunk-style record using the built-in `bytelen()` plus a custom `valueof=CRC32(...)` evaluator), each with a `go:generate` directive.
- [example_test.go](example/example_test.go) — Round-trips both: `Packet` via the no-arg `MarshalBinary`/`UnmarshalBinary`, and `Chunk` via `WriteBinaryWithMarshaler`/`ReadBinaryWithMarshaler` with a registered `CRC32` evaluator; also shows decode validation rejecting a bad value/CRC with a `*DecodeError`.

## Documentation

For full library documentation, tag syntax details, and benchmarks, see the [binarystruct README](https://github.com/mixcode/binarystruct).
