# TODO List

## Completed (v2 Release)
- [x] **One-value Marshaler/Unmarshaller**: Added `MarshalAs`, `UnmarshalAs`, `WriteAs`, and `ReadAs` to support encoding/decoding non-struct variables with explicit tags.
- [x] **Explicit Endian Marking**: Added `endian=big`, `endian=little`, and `endian=inverse` tag options to control byte order per field, propagating down into nested structs.
- [x] **Default Text Encoding Setting**: Added default text encoding control to `Marshaler`, supporting tags like `encoding=shift-jis` alongside custom text encoding registration.
- [x] **Tag Evaluator Upgrades**: Added multiplication (`*`), division (`/`), and parentheses (`()`) support to the dynamic tag expression evaluator.
- [x] **Custom Serializers**: Added support for custom encoder/decoders using the `Serializer` interface and `AddSerializer` on `Marshaler`.
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
