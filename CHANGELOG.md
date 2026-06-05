# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-06-05

A naming/API cleanup that aligns the package with Go standard-library
conventions. **This release has breaking changes** — see Changed/Removed.

### Changed (breaking)
- **`Marshaller` → `Marshaler`** (stdlib single-`l` spelling), including the
  generated `WriteBinaryWithMarshaler` / `ReadBinaryWithMarshaler` interface
  methods. No deprecated alias is shipped; for a gradual migration, downstream
  code can add its own `type Marshaller = binarystruct.Marshaler` to keep old
  *type references* compiling (the dropped `order` argument and the `Codec` /
  `codec=` renames still require call-site updates).
- **Byte order moved to construction.** `NewMarshaler(order)` (or the new
  exported `Marshaler.Order` field) sets the byte order once; the `Marshal`,
  `Unmarshal`, `Write`, `Read`, and `Inspect` **methods no longer take an
  `order` argument**. The package-level functions (`binarystruct.Marshal`,
  `Unmarshal`, `Write`, `Read`, `Append`, `Inspect`) keep their `order`
  parameter. A `Marshaler` with no byte order now fails loud with a clear error
  instead of panicking.
- **Custom `Serializer` → `Codec`.** The interface methods are renamed
  `Serialize`/`Deserialize` → `Encode`/`Decode`; `AddSerializer`/`RemoveSerializer`/
  `GetSerializer` → `AddCodec`/`RemoveCodec`/`GetCodec`; and the struct tag
  keyword **`serializer=NAME` → `codec=NAME`**.
- **Codegen requires `-endian big|little`.** The generated no-arg
  `MarshalBinary`/`UnmarshalBinary`/`AppendBinary` methods implement the
  order-less stdlib `encoding` interfaces, so the baked byte order must be chosen
  explicitly; generation errors if `-endian` is omitted (no default).
- **Minimum Go version is now 1.24** (for `encoding.BinaryAppender`).

### Added
- **`Append`** — `binarystruct.Append(buf, order, v)` and `Marshaler.Append(buf, v)`
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
