# TODO List

## Completed (v2 Release)
- [x] **One-value Marshaler/Unmarshaller**: Added `MarshalAs`, `UnmarshalAs`, `WriteAs`, and `ReadAs` to support encoding/decoding non-struct variables with explicit tags.
- [x] **Explicit Endian Marking**: Added `endian=big`, `endian=little`, and `endian=inverse` tag options to control byte order per field, propagating down into nested structs.
- [x] **Default Text Encoding Setting**: Added default text encoding control to `Marshaler`, supporting tags like `encoding=shift-jis` alongside custom text encoding registration.
- [x] **Tag Evaluator Upgrades**: Added multiplication (`*`), division (`/`), and parentheses (`()`) support to the dynamic tag expression evaluator.
- [x] **Custom Codecs**: Added support for custom encoder/decoders using the `Codec` interface and `AddCodec` on `Marshaler`.
- [x] **Benchmarks & Advanced Optimizers**: Added caching of parsed struct layout metadata to avoid reflection and tag-parsing overhead on subsequent operations.
- [x] **Omittable/Optional field tag**: Added `omittable` and `omittable=Expr` options to skip trailing or size-bounded fields on serialization and deserialization.
- [x] **Struct Inspection helper**: Added `Inspect` and layout description formatting with customizable base conversions (decimal/hex).
- [x] **Advanced Optimizers**: Implemented P-code cached static metadata, unsafe pointer interpreter engine, and layout-compatible fast-paths with vectorized endian byte-swapping supporting SIMD `simd/archsimd` fallbacks (yielding up to 214x speedups and 99.9% allocation reductions).
- [x] **JSON Layout Export & Failure Offset**: Added JSON layout serialization format for layout metadata and enhanced `DecodeError` to report the exact failure byte-offset and field name.
- [x] **Declarative Validation**: Implemented `range=min..max` and `match=pattern` validation constraints within `binary` tags, performing safe & unsafe checks during unmarshal.
- [x] **Static Code Generation (Codegen)**: Added a standalone nested module compiler CLI tool (`binarystruct-codegen`) that generates optimized static Go serialization and deserialization code, fully eliminating runtime reflection and layout interpretation.
- [x] **Codegen `bytelen()` for non-trivial fields**: Codegen now resolves `valueof=bytelen(F)` for fixed-width scalars and scalar arrays (`width*count`), fixed `string(N)` buffers (the buffer width — also fixes a latent bug that previously emitted `len`), variable text-encoded strings (an `ms`-guarded `EncodeText` measurement mirroring the encode path), and nested structs / arrays of structs (a hoisted byte-exact runtime `binarystruct.Write(io.Discard, ...)` measurement). Previously these errored out and forced the whole struct back onto the runtime interpreter. See `codegen_bytelen_test.go`.
- [x] **Codegen: conditional `tmp`/`m` scratch locals**: A generated method whose body referenced neither `tmp` nor `m` (e.g. a struct whose only field is an unbounded plain/text `string`, decoded via `io.ReadAll`) failed to compile with "declared and not used". The `var tmp [8]byte` / `var m int` declarations are now emitted only when the method body actually uses them. See `codegen_scratchvars_test.go`.
- [x] **Codegen `bytelen()` for all string variants & pointer structs**: Closed the remaining gaps — prefixed/terminated strings (`bstring`/`wstring`/`dwstring`/`zstring`/`z16string`) compute `prefixWidth + content + terminatorWidth` (mirroring the encode path), and pointer-to-struct fields measure the pointee with a nil guard (nil → 0 bytes). See `codegen_bytelen_test.go`.


## Pending / Future Ideas
- [ ] **Multidimensional arrays (Low priority)**: Support tags like `[4][2][2]int8` for nested Go slices/arrays.
- [ ] **Codegen custom `valueof` over nested-struct args (Strategy A, deferred)**: the only remaining unsupported arg shape. Would need a fully-static emit of the nested struct into a scratch buffer (retargeting the field-write emitter, plus resolving the nested struct's byte order) instead of the current `ms.MarshalAs` reuse, which can't express a nested struct's order in a standalone tag. Low priority — the runtime interpreter handles it.

<!-- EPHEMERAL (delete when the Unreleased "raw-byte fast path" + "codegen valueof via MarshalAs" entries are released) ---
Profiling (2026-06-08) that drove the two now-completed entries below.

Bench: custom CRC32(Data) over a raw []byte, vs a no-CRC control of the same wire
shape (default/unsafe build, -benchmem). Before the fieldEncodedBytes fast path:

  payload   valueof ns/op     valueof allocs/op
  64 B           30,522                  431
  1 KB          401,364                6,203
  64 KB      25,294,843              393,293
  1 MB      397,158,590            6,291,550   (~6 allocs PER BYTE)

Root cause (mem+cpu pprof): the []byte arg is encoded up to 3× per Marshal —
bytelen(Data) re-encode (to count), CRC arg re-encode (to hash), and the real
write — each a per-element reflective pass (writeArray -> encodeFunc/writeU64 with
a per-byte map dispatch). 99.94% of allocs route through fieldEncodedBytes.

Done (entry-1, rawByteRegionBytes in marshal.go): for a raw byte region hand the
bytes back directly. After: allocs CONSTANT (~41/op regardless of size); 1 MB drops
397 ms -> 0.59 ms (~676x) and 6,291,550 -> 41 allocs.

Done (entry-2, Strategy B in generator.go): a hard valueof arg is emitted as
`ms.MarshalAs(s.Field, "<reconstructed-tag>,endian=<order>")` (the runtime encoder,
byte-parity guaranteed) instead of failing loud; byte regions + integer scalars
stay inline. The tag is reconstructed by cgMarshalAsTag from cgFieldInfo (no rawTag
field needed). Only nested-struct args still fail loud (see the deferred item
above). Note: MarshalAs takes a string tag; if a struct-typed tag form is ever
needed, add an intermediate function rather than threading raw strings.
------------------------------------------------------------------------------- -->

## Completed (Unreleased)
- [x] **Codegen validation errors emit `*DecodeError` (path parity)**: generated `const`/`range`/`match` and custom-`valueof` decode validation now return a `*binarystruct.DecodeError{Offset, Field}` wrapping `ErrValidationError`, matching the runtime interpreter — so `errors.As(*DecodeError)` and the `Offset`/`Field` accessors behave identically whether a value is decoded via the interpreter or generated code. Inline checks capture the field's *start* offset (`voff<Field> := n`) before the read to match the runtime's offset exactly. See `TestCodegen_Validation_DecodeError` and `TestCodegen_CustomValueof_Validate`. (Surfaced by the custom-valueof clean-agent eval.)
- [x] **Custom `valueof` evaluators (checksums/CRC/computed trailers)**: Generalized `valueof=` beyond the built-in `bytelen()`/`count()` — register a named evaluator with `Marshaler.AddValueOf` and reference it from a tag (`valueof=CRC32(Type, Data)`), closing the F4 gap from the 0.3.0 clean-agent eval. The handler (`ValueOfFunc`) receives each referenced field's **encoded bytes** and Go value via `ValueOfContext`/`ValueOfArg`, runs on encode to produce the value, and **re-runs on decode to validate** (mismatch → `DecodeError` wrapping `ErrValidationError`; a post-decode pass, so a checksum may reference later fields). Registration is per-Marshaler (mirrors `AddCodec`); unregistered names fail loud. Multi-arg calls are enabled by a parenthesis-aware tag-option splitter (`splitTagOptions`) in both the runtime and codegen parsers. **Codegen** supports custom evaluators over any arg shape except a nested struct: byte-region fields and fixed-width integer scalars are emitted inline (byte-identical to the runtime), and every other shape (text-encoded/prefixed strings, floats, multibyte-scalar arrays, padded byte slices, variable string buffers) is re-encoded via `ms.MarshalAs` (see the codegen-valueof-via-MarshalAs entry below). Generated code needs a non-nil Marshaler (like `codec=`); decode-time validation is **on by default** (generated decode recomputes and verifies, matching the runtime). Implemented across all three paths (reflection/unsafe/codegen) with tests in every mode; SPEC/STRUCT_TAGS(+ja)/llms-full/codegen-README/AGENTS updated. See `valueof_custom_test.go` (runtime) and `codegen_customvalueof_test.go` (codegen).
- [x] **Runtime raw-byte fast path (`fieldEncodedBytes`)**: a custom `valueof`/`bytelen` over a raw byte region (byte slice/array at natural length, or an unencoded raw string) now returns the field's own bytes directly (`rawByteRegionBytes`) instead of a per-element reflective re-encode. Allocations for a checksum/length over `[]byte` become **constant** regardless of payload size (1 MB: ~6.3M allocs / ~397 ms → ~40 allocs / ~0.6 ms in the benchmark). Other shapes (text-encoded, prefixed, nested, padded) still take the exact scratch-encode path; a parity guard (`TestCustomValueof_FastPathParity_PaddedSlice`) confirms a constant-length byte slice still defers and hashes the padded bytes. Closes the first "Pending" perf follow-up.
- [x] **Codegen custom `valueof` over hard args (Strategy B, `ms.MarshalAs`)**: codegen previously failed loud for non-byte-region / non-scalar args, forcing the whole struct onto the runtime interpreter. Now every arg shape except a nested struct is supported — byte regions and integer scalars stay inline; text-encoded/prefixed strings, floats, multibyte-scalar arrays, padded byte slices, and variable string buffers are re-encoded with their own tag (reconstructed by `cgMarshalAsTag`, struct endian baked in) via `ms.MarshalAs`, the runtime encoder, so the bytes match `fieldEncodedBytes` exactly. Reflection is confined to the one (typically small) hard arg; the rest of the struct stays static. Nested-struct args still fail loud (Strategy A, deferred — see Pending). See `TestCodegen_CustomValueof_TextEncodedArg` (success + three-path parity) and `TestCodegen_CustomValueof_NestedStructArg_Errors`.
- [x] **Codegen `-no-validate` flag (unified decode-validation opt-out)**: a single `-no-validate` flag strips **all** decode-time validation from generated read methods — the `const`/`range`/`match` checks and the custom-`valueof` recompute-and-compare. Default off, so generated decode validates everything and matches the runtime interpreter exactly (full parity); set the flag for trusted-input / hot-path decoding. Replaces the earlier opt-in `-valueof-validate` flag (never released): since a custom-`valueof` struct already requires a Marshaler to encode, validating it on decode by default is symmetric and removes the prior encode-only divergence. See `TestCodegen_NoValidate_StripsBuiltins` and `TestCodegen_CustomValueof_NoValidate`.

## Completed (v0.3.0)
- [x] **Codegen: guard self-referential `bytelen` cycle (clean-agent eval finding)**: `binarystruct-codegen` previously **stack-overflowed (fatal crash)** on `NameLen uint16 \`binary:"uint16,valueof=bytelen(Name)"\`` + `Name string \`binary:"string(NameLen)"\`` — `bytelenExpr` resolved the `string(N)` width to `N` (= `NameLen`), but `NameLen` is the `valueof` field, so `bytelenExpr` ↔ `translateEncodeExpr`/`translateValueof` recursed forever. Now a `visiting` set of in-progress valueof fields detects the re-entry and emits a clear generation error ("self-referential valueof/bytelen cycle … use the runtime interpreter"). The runtime interpreter still handles the shape; the canonical `[NameLen]byte` shape is unaffected. See `TestCodegen_BytelenCycle_Errors`.
