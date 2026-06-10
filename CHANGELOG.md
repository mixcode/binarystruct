# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Documentation
- **`llms.txt`: added a `## Workspace (modules)` map** — a two-row table (the root
  `binarystruct` library and the `binarystruct-codegen` CLI module) plus
  build/test/install notes, so an agent landing at the repo root sees both
  independently-published modules and how to work with each. The library index is
  unchanged. (From an `agent-friendly-guide` monorepo survey.)

## [0.3.5] - 2026-06-10

### Performance
- **Codegen: contiguous fixed-width scalar fields are batched** into one shared
  buffer + a single `Write` (decode: one `ReadFull` + per-field parse), instead of
  one `Write`/`ReadFull` per field. A run of ≥2 plain scalar fields (no
  `valueof`/`const`/`range`/`match`/`codec`/`omittable`/array/per-field endian) is
  coalesced; runs are broken by any such field. **Byte-identical** to the
  per-field path (no `unsafe`, fully portable) — the win is fewer io calls, not
  fewer allocations. Measured ~1.5× Marshal and Unmarshal on a 10-scalar header.
  Always on; no flag. (TODO #2 from the 0.3.2 codegen profiling pass.)

### Changed
- **Codegen: the "no byte order" error now names an ordered parent.** When a type
  with no declared order is generated in isolation but is referenced as a field by
  another struct that declares one (the order it would inherit at runtime), the
  error names that parent and the exact fix — e.g. *"… it is used by Container,
  which is big-endian; pass `-endian big`, or declare the order on Record itself."*
  A truly orphan type still gets the plain error. (Surfaced by the 0.3.3 clean-agent eval.)

### Documentation
- **Fixed a stale godoc claim that codegen *requires* `-endian`.** The package doc
  (`doc.go`) and the codegen command doc (`binarystruct-codegen` package comment)
  said `-endian` was required when generating Go code; it has been the optional
  *fallback* (a struct's own `endian=` declaration wins) since 0.3.0. Both now say
  so, the codegen godoc flag list is completed (`-json`/`-tests`/`-no-validate`/`-unsafe-bulk`),
  and a dead "missing required -endian" code branch was removed.
- **`binarystruct-codegen/README.md`**: the install section now uses `@latest` as
  the only concrete command and documents the path-prefixed nested-module pin
  syntax generically (`@binarystruct-codegen/vX.Y.Z`) instead of a concrete version,
  so the example no longer needs a per-release bump.
- **`TODO.md`** trimmed to a forward-looking list — completed entries dropped
  (the CHANGELOG + git history are the record); the stale "up to 214x" optimizer
  figure now points to the generated README benchmark table.

## [0.3.4] - 2026-06-09

Documentation only — **no code or wire-format changes**. Agent-readiness polish
triaged from the v0.3.3 clean-agent evaluation (which scored 5/5 with a
fully-working build).

### Documentation
- **`AGENTS.txt`**: added an "audience: contributors" header that points library
  *users* to `llms-full.txt` (the consumer manual) — `AGENTS.txt` is the
  contributor guide and was briefly opened by mistake during the eval.
- **`llms-full.txt`**: added a combined end-to-end recipe — a chunked container
  (struct-declared order + `const` magic + `valueof` count/length + a custom CRC +
  a count-prefixed slice-of-struct via bare `[Count]`), since those pieces were
  previously scattered across Rule E / §7 / §8. Also documented the Go
  nil-vs-empty `[]byte` round-trip gotcha (compare with `bytes.Equal`, not
  `reflect.DeepEqual`).
- **`binarystruct-codegen/README.md`**: documented the path-prefixed nested-module
  install tag (`@binarystruct-codegen/vX.Y.Z`) and the `$GOBIN` install location.

## [0.3.3] - 2026-06-09

### Added
- **Real SIMD byte-swap kernel.** Under `-tags experiment_simd` (`GOEXPERIMENT=simd`)
  on amd64 (Go 1.26+), `swapBytes` now uses an AVX2 `VPSHUFB` shuffle instead of the
  previous conceptual stub; non-amd64 / non-experiment builds keep the scalar
  fallback. Measured on a swap-heavy big-endian workload: runtime unsafe Marshal
  ~1.26×, Unmarshal ~1.84×. The kernel is amd64-only **by design** because Go's
  `simd/archsimd` is amd64-only today — see the upstream-tracking note in `AGENTS.txt`.
- **Exported `binarystruct.SwapBytes(buf, width)` and `binarystruct.HostEndian()`**,
  so generated code can share the (SIMD-capable) byte-swap path.
- **Codegen `-unsafe-bulk` flag (default off).** For fixed-width scalar
  arrays/slices whose Go element width matches the wire width, the generator emits a
  raw-memory bulk path (one `Write`/`ReadFull` over the element backing store via
  `unsafe`, plus one in-place `SwapBytes` when the order differs from the host) that
  inherits the SIMD acceleration under `experiment_simd`. **Byte-identical** to the
  default per-element path — a portability-for-speed knob, not a wire-format change.
  Generated output is unchanged unless the flag is set (no `unsafe` import by default).

### Performance
- With `-unsafe-bulk`, generated big-endian `[1000]uint32`: Unmarshal **4,848 → 1,334 ns**
  (default build) and **→ 273 ns** under `experiment_simd`; allocations drop from
  4,144 B/2 to 48 B/1. Little-endian (host-order) Unmarshal **→ 111 ns** via the
  zero-copy read.

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
