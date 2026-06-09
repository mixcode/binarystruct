# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.2] - 2026-06-09

A performance release — **no API changes**.

### Performance
- **Runtime: no per-scalar heap allocation.** `read`/`writeU64` stage through a
  reusable Marshaler-owned scratch buffer instead of a fresh per-call array, so the
  staging slice no longer escapes to the heap via `io.Writer.Write` / `io.ReadFull`.
- **Runtime: constant tag expressions resolved once.** Constant array/buffer
  lengths (`string(10)`, `[1000]uint32`) are pre-resolved at metadata time instead
  of being re-tokenized on every operation.
- **Runtime: encode/decode scalar closures cached** by (Go type, binary type) pair,
  removing a per-element closure allocation.
- **Runtime: bulk-buffer scalar slices (safe mode).** A fixed-width scalar
  array/slice encodes/decodes through one contiguous buffer + a single
  `Write`/`ReadFull` instead of N per-element calls. Combined effect: safe-mode
  slice allocations go from ~2–3 per element to O(1) (e.g. unmarshalling a
  1000-element slice: ~3005 allocs → ~4, ~8× faster); unsafe small-struct marshal
  19 → 6 allocs.
- **Codegen: bulk-buffer scalar slices.** Generated code for a multibyte
  fixed-width scalar array/slice now fills one buffer + single `Write` (decode: one
  `ReadFull` + parse) instead of a per-element loop (~2.6–2.9× faster).
- **Codegen: nested generated types are called directly.** A nested struct (or
  struct-slice element) whose type is itself generated is encoded/decoded with a
  direct `WriteBinaryWithMarshaler`/`ReadBinaryWithMarshaler` call instead of the
  reflection runtime with a per-element `Marshaler` allocation (~4.4× faster). This
  also passes the `Marshaler` through, so a **nested `codec=`/`encoding=`/custom
  `valueof` now receives its registered context** (previously silently dropped).

### Note
- A `*Marshaler` must not be shared across goroutines — it now also carries a
  reusable scalar scratch buffer. Use one instance per goroutine, or the
  package-level functions. (This matches the pre-existing rule for its
  lazily-populated encoder cache; independent Marshalers remain safe to run
  concurrently.)

## [0.3.1] - 2026-06-09

### Added
- **Custom `valueof` evaluators** for derived fields the built-ins can't express
  (checksums, CRCs, computed trailers). Register a named evaluator on a Marshaler
  with `AddValueOf(name, func(ValueOfContext) (uint64, error))` and reference it
  from a tag — `valueof=CRC32(Type, Data)`. The evaluator receives each referenced
  field's **encoded bytes** (and Go value) via `ValueOfContext`, runs on encode to
  produce the value, and **re-runs on decode to validate** it (mismatch →
  `DecodeError` wrapping `ErrValidationError`, naming the field; a post-decode pass,
  so a checksum may reference later fields). New API: `Marshaler.AddValueOf` /
  `RemoveValueOf` / `GetValueOf`, and the `ValueOfFunc` / `ValueOfContext` /
  `ValueOfArg` types. Registration is per-Marshaler (like custom `Codec`s), so the
  package-level functions don't see it; an unregistered name fails loud.
- The `valueof` tag now accepts **multi-argument** function calls; the binary-tag
  option splitter is parenthesis-aware, so commas inside a call's argument list no
  longer split the option list. (The built-in `bytelen`/`count` remain single-arg.)
- **Codegen support for custom `valueof` evaluators** over **any argument shape
  except a nested struct**. Byte-region args (`[]byte`/`[N]byte`, raw `string`,
  constant-size `string(N)` without text encoding) and fixed-width integer scalars
  (`order.PutUintN`) are emitted inline; every other shape (text-encoded or
  prefixed/terminated strings, floats, multibyte-scalar arrays, padded byte slices,
  variable string buffers) is re-encoded with its own tag via `ms.MarshalAs`, so
  the bytes match the runtime exactly (verified byte-for-byte). Generated code
  requires a non-nil Marshaler (like `codec=`; call `WriteBinaryWithMarshaler`).
  Decode-time validation is **on by default** — generated decode recomputes the
  value and, on mismatch, returns a `*DecodeError` wrapping `ErrValidationError`
  (offset + field), matching the runtime interpreter.
- **`-no-validate` codegen flag** strips **all** decode-time validation from the
  generated read methods — the `const`/`range`/`match` checks and the custom-
  `valueof` recompute-and-compare. Default off (generated decode validates
  everything, matching the runtime); set it for trusted-input / hot-path decoding.
  (Replaces the earlier `-valueof-validate` opt-in flag, which was never released:
  decode validation is now on by default and `-no-validate` is the single opt-out
  for all decode checks.)

- **Multidimensional array tags** — stack length prefixes (`[2][3]int16`,
  `[2][2][2]int8`) to encode/decode nested Go arrays and slices in row-major order.
  Each dimension is an independent expression, so dimensions may reference other
  fields (`[Rows][Cols]uint8`); slice levels are allocated to the declared lengths
  on decode, and any leaf type works (scalars, strings, nested structs).
  `binarystruct-codegen` generates multidimensional arrays with a **scalar leaf
  type** (fixed arrays, and slices with all dimensions specified, byte-identical to
  the runtime); non-scalar leaves (strings, nested structs, pointers) or mixed
  fixed-array/slice nesting fail generation (use the runtime interpreter).

### Performance
- **Raw-byte fast path in the runtime `valueof`/`bytelen` measurement.**
  `fieldEncodedBytes` now hands back a raw byte region (byte slice/array at natural
  length, or an unencoded raw string) directly instead of re-encoding it
  element-by-element through reflection. A checksum or `bytelen` over a `[]byte`
  now allocates a **constant** amount regardless of payload size (previously ~linear
  in the byte count): a 1 MB payload dropped from ~6.3M allocations to ~40, and
  from ~397 ms to ~0.6 ms per encode in the benchmark. Other shapes are unchanged.

### Fixed
- **Untagged nested Go arrays no longer encode to zero bytes.** A struct field of
  a nested array/slice type (e.g. `[2][3]int16`) with no `binary` tag previously
  encoded as nothing (an outer length of 0 was inferred and the field silently
  skipped); it is now encoded by natural inference.
- **Codegen validation errors now match the runtime's type.** Generated
  `const`/`range`/`match` and custom-`valueof` decode-validation failures return a
  `*DecodeError{Offset, Field}` wrapping `ErrValidationError` (previously a plain
  `fmt.Errorf`), so `errors.As(&DecodeError)` and the `Offset`/`Field` accessors
  behave the same whether a value is decoded by the interpreter or by generated
  code. Inline checks report the field's start offset, as the runtime does.

### Limitations
- Codegen rejects a custom `valueof` evaluator whose referenced arg is a
  **nested struct** (its byte order can't be expressed in a standalone `MarshalAs`
  tag) — generation fails loud; use the runtime interpreter for those structs.
  (Alongside the existing codegen exclusions: struct-level `endian=inverse` and
  order/encoding inheritance via embedding.)

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

[0.3.2]: https://github.com/mixcode/binarystruct/releases/tag/v0.3.2
[0.3.1]: https://github.com/mixcode/binarystruct/releases/tag/v0.3.1
[0.3.0]: https://github.com/mixcode/binarystruct/releases/tag/v0.3.0
[0.2.6]: https://github.com/mixcode/binarystruct/releases/tag/v0.2.6
[0.2.5]: https://github.com/mixcode/binarystruct/releases/tag/v0.2.5
[0.2.4]: https://github.com/mixcode/binarystruct/releases/tag/v0.2.4
