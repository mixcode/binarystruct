# TODO List

## Completed (v2 Release)
- [x] **One-value Marshaller/Unmarshaller**: Added `MarshalAs`, `UnmarshalAs`, `WriteAs`, and `ReadAs` to support encoding/decoding non-struct variables with explicit tags.
- [x] **Explicit Endian Marking**: Added `endian=big`, `endian=little`, and `endian=inverse` tag options to control byte order per field, propagating down into nested structs.
- [x] **Default Text Encoding Setting**: Added default text encoding control to `Marshaller`, supporting tags like `encoding=shift-jis` alongside custom text encoding registration.
- [x] **Tag Evaluator Upgrades**: Added multiplication (`*`), division (`/`), and parentheses (`()`) support to the dynamic tag expression evaluator.
- [x] **Custom Serializers**: Added support for custom encoder/decoders using the `Serializer` interface and `AddSerializer` on `Marshaller`.
- [x] **Benchmarks & Advanced Optimizers**: Added caching of parsed struct layout metadata to avoid reflection and tag-parsing overhead on subsequent operations.

## Pending / Future Ideas
- [ ] **Omittable/Optional field tag**: Add support for `omittable` or `optional` tag options, especially for checking end-of-struct by comparing current offset against a dynamic struct size.
- [ ] **Multidimensional arrays**: Support tags like `[4][2][2]int8` for nested Go slices/arrays.
- [ ] **Advanced Optimizers**: Precompiled P-code based encoder/decoder, or SIMD-assisted type/endian conversions.
- [ ] **Struct Inspection helper**: A function to print offsets and sizes of struct fields for debugging layouts.
