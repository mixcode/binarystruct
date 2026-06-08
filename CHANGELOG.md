# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-06-08

A naming/API cleanup that aligns the package with Go standard-library
conventions. **This release has breaking changes** — see Changed/Removed.

### Changed (breaking)
- **`Marshaller` → `Marshaler`** (stdlib single-`l` spelling), including the
  generated `WriteBinaryWithMarshaler` / `ReadBinaryWithMarshaler` interface
  methods. No deprecated alias is shipped; for a gradual migration, downstream
  code can add its own `type Marshaller = binarystruct.Marshaler` to keep old
  *type references* compiling (the dropped `order` argument and the `Codec` /
  `codec=` renames still require call-site updates).
- **Byte order is now declared on the struct, and the API is order-free.** A
  struct states its byte order with a blank `_ struct{}` field tagged
  `binary:"endian=big|little"` (or by embedding a struct that declares one); that
  declaration is authoritative. The package functions and the `Marshaler` methods
  (`Marshal`, `Unmarshal`, `Write`, `Read`, `Append`, `Inspect`) **no longer take
  an `order` argument**. Order resolution, most specific first: a per-field
  `endian=` tag → the struct's declaration → the Marshaler's `Order` field
  (fallback, set via `NewMarshalerOrder`) → otherwise encoding/decoding a
  multi-byte value fails loud with a clear error.
- **Constructors:** `NewMarshaler()` (no fallback order) and
  `NewMarshalerOrder(order)` (fallback order for values that declare none, e.g.
  bare scalars or third-party structs). The previous `NewMarshaler(order)` is
  replaced.
- **Custom `Serializer` → `Codec`.** The interface methods are renamed
  `Serialize`/`Deserialize` → `Encode`/`Decode`; `AddSerializer`/`RemoveSerializer`/
  `GetSerializer` → `AddCodec`/`RemoveCodec`/`GetCodec`; and the struct tag
  keyword **`serializer=NAME` → `codec=NAME`**.
- **Codegen `-endian` is now optional.** A struct's own `endian=` declaration
  supplies (and overrides) the order baked into the generated no-arg
  `MarshalBinary`/`UnmarshalBinary`/`AppendBinary`; `-endian big|little` is only
  the fallback for structs that declare none. Generation **errors** if neither
  the struct nor the flag provides an order (no default). Codegen does not support
  struct-level `endian=inverse` or order inheritance via embedding (use the
  runtime interpreter).
- **Minimum Go version is now 1.24** (for `encoding.BinaryAppender`).

### Added
- **Struct-level options** via the blank `_ struct{}` sentinel (and via
  value-embedding a declaring struct; conflicting inherited values are an error):
  `endian=` (the struct's byte order) and `encoding=` (a default text encoding for
  its string fields, between a per-field `encoding=` and `Marshaler.DefaultTextEncoding`).
  Codegen supports both struct-level `endian=` and `encoding=` (only inheritance
  via embedding is codegen-unsupported).
- **`Append`** — `binarystruct.Append(buf, v)` and `Marshaler.Append(buf, v)`
  encode a value and append it to a buffer (the `encoding/binary.Append` analog).
- **Codegen emits `AppendBinary`** implementing `encoding.BinaryAppender` (Go 1.24).
- An agent-facing example showing a tagged type implement
  `encoding.BinaryMarshaler`/`BinaryUnmarshaler`/`BinaryAppender` via binarystruct,
  including the method-less-twin trick that avoids infinite recursion.

## [0.2.6] - 2026-06-04

### Added
- **Code generation now resolves `valueof=bytelen(F)` for (almost) every field
  shape.** Previously `binarystruct-codegen` only handled byte-slices and raw
  strings and errored on anything else, forcing the whole struct back onto the
  runtime interpreter. Now supported:
  - fixed-width scalars and scalar arrays (`width × count`, computed statically);
  - fixed `string(N)` buffers (the buffer width `N`);
  - all variable / length-prefixed / null-terminated string variants
    (`string`, `bstring`, `wstring`, `dwstring`, `zstring`, `z16string`),
    computed as `prefix + content + terminator`, with an `ms`-guarded
    `EncodeText` measurement for text-encoded content so the byte count matches
    the encoded form;
  - nested structs and arrays of structs (a byte-exact runtime measurement that
    mirrors the encode path);
  - pointer-to-struct fields (a `nil` pointer contributes `0` bytes).

### Fixed
- **Codegen no longer emits unused `tmp`/`m` scalar scratch variables.** A
  generated method whose body referenced neither (e.g. a struct whose only field
  is an unbounded string, decoded via `io.ReadAll`) previously failed to compile
  with "declared and not used". The declarations are now emitted only when used.
- Fixed a latent codegen bug where `bytelen()` of a fixed `string(N)` emitted
  `len(field)` instead of the buffer width `N`.

### Internal
- Codegen integration tests build the generator once (in `TestMain`) and run in
  parallel, cutting the package test time from ~7s to ~1.3s.

## [0.2.5] - 2026-06-03

### Added
- **`const` tag** for fixed/magic values (e.g. file signatures). The value is
  emit-only on encode and validated against the stream on decode, returning
  `ErrValidationError` on mismatch. Supports both integer and byte-sequence
  magics.
- The `range=min..max` constraint and other size/length tag expressions now
  accept hexadecimal literals (`0x…`) and arithmetic.

## [0.2.4] - 2026-06-03

### Added
- **`valueof` tag** for encode-time computed integer fields. A length/count
  field can be derived from other fields via `bytelen(F)` and `count(F)`
  combined with arithmetic (e.g. `valueof=bytelen(Name)`). It is emit-only: the
  computed value is written to the stream but the Go field is left unmodified.

## [0.2.0 – 0.2.3] - 2026-06-01 … 2026-06-02

This series introduced the major feature wave (entries consolidated, as they
predate this changelog):

### Added
- **Static code generation** — the standalone `binarystruct-codegen` CLI/module
  emits optimized Go marshal/unmarshal code, eliminating runtime reflection and
  layout interpretation; includes `-json` layout export and `-tests` support.
- **Declarative validation** — `range=min..max` and `match=pattern` constraints
  checked during unmarshal.
- **JSON layout export** and enhanced `DecodeError` reporting the exact failure
  byte-offset and field name.
- **Advanced optimizers** — cached parsed struct metadata, an unsafe-pointer
  interpreter engine, layout-compatible slice fast-paths, and vectorized
  byte-swapping with a portable fallback.
- Agent-readiness docs (`llms.txt`, `llms-full.txt`, `AGENTS.txt`) and the
  struct-tag reference manual integrated into `doc.go`.

### Changed
- Renamed the build tag `safe` → `safe_binarystruct` to avoid collisions.

[0.3.0]: https://github.com/mixcode/binarystruct/releases/tag/v0.3.0
[0.2.6]: https://github.com/mixcode/binarystruct/releases/tag/v0.2.6
[0.2.5]: https://github.com/mixcode/binarystruct/releases/tag/v0.2.5
[0.2.4]: https://github.com/mixcode/binarystruct/releases/tag/v0.2.4
