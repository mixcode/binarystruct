# TODO List

## Completed (v2 Release)
- [x] **One-value Marshaller/Unmarshaller**: Added `MarshalAs`, `UnmarshalAs`, `WriteAs`, and `ReadAs` to support encoding/decoding non-struct variables with explicit tags.
- [x] **Explicit Endian Marking**: Added `endian=big`, `endian=little`, and `endian=inverse` tag options to control byte order per field, propagating down into nested structs.
- [x] **Default Text Encoding Setting**: Added default text encoding control to `Marshaller`, supporting tags like `encoding=shift-jis` alongside custom text encoding registration.
- [x] **Tag Evaluator Upgrades**: Added multiplication (`*`), division (`/`), and parentheses (`()`) support to the dynamic tag expression evaluator.
- [x] **Custom Serializers**: Added support for custom encoder/decoders using the `Serializer` interface and `AddSerializer` on `Marshaller`.
- [x] **Benchmarks & Advanced Optimizers**: Added caching of parsed struct layout metadata to avoid reflection and tag-parsing overhead on subsequent operations.
- [x] **Omittable/Optional field tag**: Added `omittable` and `omittable=Expr` options to skip trailing or size-bounded fields on serialization and deserialization.
- [x] **Struct Inspection helper**: Added `Inspect` and layout description formatting with customizable base conversions (decimal/hex).
- [x] **Advanced Optimizers**: Implemented P-code cached static metadata, unsafe pointer interpreter engine, and layout-compatible fast-paths with vectorized endian byte-swapping supporting SIMD `simd/archsimd` fallbacks (yielding up to 214x speedups and 99.9% allocation reductions).
- [x] **JSON Layout Export & Failure Offset**: Added JSON layout serialization format for layout metadata and enhanced `DecodeError` to report the exact failure byte-offset and field name.
- [x] **Declarative Validation**: Implemented `range=min..max` and `match=pattern` validation constraints within `binary` tags, performing safe & unsafe checks during unmarshal.
- [x] **Static Code Generation (Codegen)**: Added a standalone nested module compiler CLI tool (`codegen`) that generates optimized static Go serialization and deserialization code, fully eliminating runtime reflection and layout interpretation.


## Pending / Future Ideas
- [ ] **Multidimensional arrays (Low priority)**: Support tags like `[4][2][2]int8` for nested Go slices/arrays.
