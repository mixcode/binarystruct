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
- [ ] **Custom `valueof` evaluators (checksums/CRC/computed trailers)**: Generalize `valueof=` beyond the built-in `bytelen()`/`count()` so a user can register a named evaluator and reference it from a tag — closing the F4 gap from the clean-agent eval (a checksum is the most common *non-length* derived field, and is currently the one place "derive the field from the data" stops). Motivating case: PNG's `CRC32` over `Type`+`Data`.

  **Surface (instance-scoped, mirrors `AddCodec`):**
  ```go
  ms.AddValueOf("CRC32", func(c binarystruct.ValueOfContext) (uint64, error) {
      h := crc32.NewIEEE()
      for _, a := range c.Args { h.Write(a.Bytes) }
      return uint64(h.Sum32()), nil
  })
  // field tag:  `binary:"uint32,valueof=CRC32(Type,Data)"`
  ```
  Registered on the `Marshaler` (not a package global) for symmetry with custom codecs and to avoid global mutable state — so, like codecs, custom `valueof` requires a configured `Marshaler` instance (the package-level funcs use the default one). `bytelen`/`count` stay special-cased built-ins (statically codegen-able); the hook is purely additive.

  **Handler context** — gives both encoded bytes (the correctness keystone) and Go values:
  ```go
  type ValueOfContext struct {
      Struct   any           // pointer to the (de)serialized struct
      Target   string        // name of the field being computed, e.g. "CRC"
      Args     []ValueOfArg  // referenced members, in tag order
      Decoding bool           // false=encode, true=decode-validation
  }
  type ValueOfArg struct {
      Name  string
      Bytes []byte // ENCODED bytes (honors byte order / text encoding / width) — what a checksum needs
      Value any    // the Go value, for sum/xor-of-fields style math
  }
  type ValueOfFunc func(ctx binarystruct.ValueOfContext) (uint64, error)
  ```
  Returns an integer, written per the field's declared binary type. **Must operate on `Args[].Bytes`, not Go values**, because byte order, text encodings (Shift-JIS), and type width change what actually hits the stream — a CRC over the in-memory value would silently disagree with the file.

  **Multi-argument grammar (decided: paren-aware comma).** Today the option splitter is a flat `strings.Split(tagStr, ",")` (`struct.go:529`/`:646`), so `CRC32(Type,Data)` would mis-tokenize; and the SPEC currently reserves multi-arg ("Functions take exactly one field-name argument in this version; multi-argument forms are reserved", line 73). Plan: make the comma split **paren-aware** (split on commas only at paren depth 0) in *both* tag parsers (runtime `struct.go` **and** codegen `generator.go`), so `valueof=CRC32(Type, Data)` reads like a natural Go call. This is backward-compatible in practice — no existing tag has a comma inside parens (`(buf_len)`/`[arrayLen]` hold arithmetic only). Then lift the SPEC reservation.

  **Decode = validation.** Unlike the emit-only `bytelen`, a custom evaluator also runs on **decode**: recompute over the read bytes and compare to the value read from the stream; mismatch → `DecodeError` wrapping `ErrValidationError` (offset + field), exactly like `const`. `Decoding=true` lets a handler opt out. (A checksum that is written but never verified is half a feature.) Emit-only write-back stays consistent with `bytelen`/F5: after `Marshal` the in-memory field keeps its old value.

  **Memory/CPU.** Bounded by the largest referenced member, not the struct or stream — reuse the existing `bytelen` scratch-encode mechanism to materialize each referenced member's bytes on demand (streaming `Write` stays streaming; forward references re-derive rather than reach back). Raw `[]byte`/byte-array members (the typical target, e.g. PNG `Data`) need **no** scratch encode — hand the slice directly; only byte-swapped scalars / text-encoded strings / nested structs re-encode. The real cost is a possible double/triple-encode of covered fields (CPU, not RAM). Optional later optimization: a "capturing writer" that tees a contiguous covered range during the real write to avoid re-encoding — defer until profiling justifies it.

  **Three-path parity.** Reflection + unsafe paths call the registered closure at runtime. **Codegen cannot embed a runtime closure**, so the first cut **fails loud** on custom `valueof` (consistent with how codegen already rejects `endian=inverse` and embedding-inheritance — the runtime interpreter still handles it). Follow-up: a name→function mapping flag (e.g. `-valueof "CRC32=github.com/me/png.CRC"`) so codegen can emit a direct qualified call and keep the generated code self-contained.

  **Extension checklist when implemented:** runtime (`marshal.go` encode + `unmarshal.go` decode-validation), unsafe (`unsafe_io.go`, likely routed through the reflection writer like `valueof`/`const` already are), codegen fail-loud guard (`generator.go`), the paren-aware split in both parsers, SPEC update (lift the multi-arg reservation; document decode-validation + the bytes-not-values rule), `STRUCT_TAGS{,_ja}.md`, `llms-full.txt` §7 recipe (the F4 fix), and tests in all modes (a PNG-CRC round-trip + a deliberate-mismatch validation-error test).

- [ ] **Multidimensional arrays (Low priority)**: Support tags like `[4][2][2]int8` for nested Go slices/arrays.

## Completed (v0.3.0)
- [x] **Codegen: guard self-referential `bytelen` cycle (clean-agent eval finding)**: `binarystruct-codegen` previously **stack-overflowed (fatal crash)** on `NameLen uint16 \`binary:"uint16,valueof=bytelen(Name)"\`` + `Name string \`binary:"string(NameLen)"\`` — `bytelenExpr` resolved the `string(N)` width to `N` (= `NameLen`), but `NameLen` is the `valueof` field, so `bytelenExpr` ↔ `translateEncodeExpr`/`translateValueof` recursed forever. Now a `visiting` set of in-progress valueof fields detects the re-entry and emits a clear generation error ("self-referential valueof/bytelen cycle … use the runtime interpreter"). The runtime interpreter still handles the shape; the canonical `[NameLen]byte` shape is unaffected. See `TestCodegen_BytelenCycle_Errors`.
