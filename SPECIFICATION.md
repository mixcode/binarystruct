# Specification & Compiler Alignment Guide (Ground Truth)

This document serves as the ground-truth specification for the `binarystruct` tag syntax, runtime serialization semantics, and static code generation (codegen) compiler mapping. 

Any extension to the tag syntax, type options, or serialization logic **must** implement and align both the dynamic runtime path and the static code generator compiler path according to the rules defined below.

---

## 1. Ground Truth Specification & Semantics

| Binary Tag Type | Go Kind Representation | Encoded Width | Runtime Interpreter Logic | Codegen Compiler Mapping |
| :--- | :--- | :--- | :--- | :--- |
| **`int8`** / **`uint8`** / **`byte`** | `int8` / `uint8` / `bool` / `byte` | 1 byte | Reads/writes 1 byte. | `tmp[0] = byte(val)` / `val = tmp[0]` |
| **`int16`** / **`uint16`** / **`word`** | Signed/unsigned 16-bit | 2 bytes | Reads/writes 2 bytes; applies endianness. | `order.PutUint16(...)` / `order.Uint16(...)` |
| **`int32`** / **`uint32`** / **`dword`** | Signed/unsigned 32-bit | 4 bytes | Reads/writes 4 bytes; applies endianness. | `order.PutUint32(...)` / `order.Uint32(...)` |
| **`int64`** / **`uint64`** / **`qword`** | Signed/unsigned 64-bit | 8 bytes | Reads/writes 8 bytes; applies endianness. | `order.PutUint64(...)` / `order.Uint64(...)` |
| **`float32`** | `float32` | 4 bytes | IEEE 754 float32 mapping. | `math.Float32bits(...)` / `math.Float32frombits(...)` |
| **`float64`** | `float64` | 8 bytes | IEEE 754 float64 mapping. | `math.Float64bits(...)` / `math.Float64frombits(...)` |
| **`pad(size)`** | None | `size` bytes | Skips bytes on read; writes zero bytes on write. | `w.Write(make([]byte, size))` / `io.ReadFull(r, make([]byte, size))` |
| **`string(size)`** | `string` | `size` bytes | Raw string. Padded with `0` on write; trimmed on read. | `copy(writeBytes, stringBytes)` / `strlen := len(strBytes); for ; strlen > 0 && strBytes[strlen-1] == 0; strlen-- {}` |
| **`bstring`** / **`wstring`** / **`dwstring`** | `string` | 1/2/4 + len bytes | Length-prefixed string. Width of prefix defined by prefix type. | Writes/reads prefix width as integer, then writes/reads string bytes. |
| **`zstring`** | `string` | len + 1 bytes | Null-terminated C-style string. | Writes string + `0`; reads until `0` byte. |
| **`z16string`** | `string` | 2*len + 2 bytes | Null-word-terminated UTF-16 style string. | Writes string + `0x0000`; reads until `0x0000`. |
| **`ignore`** / **`-`** | Any | 0 bytes | Bypassed. | Bypassed. |
| **`any`** | Any | Natural | Resolves to Go field's natural primitive type mapping. | Bypassed or resolved to the primitive. |
| **`custom`** | Any | Custom | Requires `codec` option. Delegates to custom Codec. | Looks up codec from Marshaler context via `GetCodec()`; calls Encode/Decode. |

### Tag Options

Tag options modify the behavior of binary types. They are appended after the type, e.g. `` `binary:"uint16,endian=big"` ``.

| Option | Syntax | Applies To | Description |
| :--- | :--- | :--- | :--- |
| **`endian`** (struct-level) | `endian=big\|little` on a blank `_ struct{}` field | The whole struct | Declares the struct's byte order (see §2). Propagates to all fields and nested structs; inherited via embedding. The sentinel encodes to 0 bytes. |
| **`endian`** (per-field) | `endian=big\|little\|inverse` | Integer/float types | Per-field **override** of the struct's declared order; `inverse` flips the inherited order. Needed only on fields that differ — not on every field. |
| **`encoding`** | `encoding=NAME` | String types | Applies a text encoding (e.g. Shift-JIS) registered in the Marshaler. |
| **`codec`** | `codec=NAME` | `custom` type | Specifies which registered Codec to delegate to. |
| **`omittable`** | `omittable` or `omittable=Expr` | Any | Allows truncated streams: if EOF is reached at this field's start, decoding stops without error. |
| **`range`** | `range=min..max` | Numeric types | Validates deserialized value is within `[min, max]`. Returns error on violation. |
| **`match`** | `match=pattern` | String types | Validates deserialized string matches the regex pattern. Returns error on violation. |
| **`valueof`** | `valueof=Expr` | Integer/bitmap types | **Encode-only.** Computes the field's serialized value from an expression (may use `bytelen()`/`count()`). Emit-only: the Go field is not modified. See [Computed Field Assignment](#computed-field-assignment-valueof-bytelen-count). |
| **`const`** | `const=Value` | Integer/bitmap or raw byte sequence | **Encode + decode.** Emits a fixed value (emit-only; field ignored) and validates it on decode (`ErrValidationError` on mismatch). Integer = constant int expression (endian-sensitive); byte sequence = natural-order hex blob. See [Fixed / Magic Values](#fixed--magic-values-const). |

### Computed Field Assignment: `valueof`, `bytelen()`, `count()`

The `valueof` option auto-computes an integer field's **serialized** value from other fields during encoding, removing manual length/count bookkeeping (e.g. a filename-length field that must equal `len(Name)`).

**Direction — encode-only.** `valueof` is evaluated by `Marshal`/`Write` only. On decode it is **ignored**: the field is read from the stream as a normal integer. A `valueof` length field is therefore paired with a decode-side size expression on its target field, the two being inverses:

```go
NameLen uint16 `binary:"uint16,valueof=bytelen(Name)"` // encode: written as len(Name)
Name    []byte `binary:"[NameLen]byte"`               // decode: sized from NameLen
```

**Emit-only (no write-back).** The computed value is written to the byte stream; the Go struct field is **not** modified, and any value the caller placed in it is ignored on encode. This keeps encoding side-effect-free and the output stream strictly forward-only — the value is derived by measuring referenced fields into a scratch buffer, never by seeking back to patch the stream.

> **Design decision (permanent): write-back will never be implemented.** A "write-back" mode that stored computed values back into the Go struct was considered and **deliberately rejected**, for reasons that outweigh the small amount of code it would take:
> 1. **Encoding must stay a pure read.** Emit-only `Marshal`/`Write` never mutate their input, so the same value (or a shared pointee) can be marshalled concurrently without a data race. Write-back would turn encoding into a mutation and silently introduce that race — an unacceptable regression for a serialization library.
> 2. **No pointer-vs-value contract surprise.** Reflective write-back only works when the caller passed an addressable (pointer) argument; a plain value could not be updated. Emit-only behaves identically regardless of how the argument was passed.
> 3. **One behavior across all three paths.** Safe, unsafe, and codegen paths stay byte-identical with no per-path settability edge cases to keep in sync.
>
> **If you need the struct populated with the computed values, perform a `Marshal`/`Unmarshal` (or `Write`/`Read`) round trip:** marshal the struct to bytes, then unmarshal those bytes back into a struct of the same type. On decode the length/count fields are read from the stream normally, so the resulting struct carries the true serialized values.

**Applies to:** integer and bitmap target types (`int8`…`int64`, `uint8`…`uint64`, `byte`/`word`/`dword`/`qword`). `valueof` on any other field type is a compile-time (metadata) error.

**Expression grammar.** `valueof` reuses the standard arithmetic evaluator (integer literals incl. `0x`/`0o`/`0b`, `+ - * /`, parentheses, and field references) and extends it with **single-argument functions**:

| Function | Result |
| :--- | :--- |
| **`bytelen(F)`** | Total **encoded byte size** of field `F` (exact: honors text encodings, length prefixes, arrays, and nested structs). Valid for any field. |
| **`count(F)`** | **Element count** (`len(F)`) of an array or slice field `F`. Not valid for strings (no unambiguous element count under text encodings) — use `bytelen` for a string's byte length. |

Examples: `valueof=bytelen(Name)`, `valueof=bytelen(Payload)+2`, `valueof=count(Items)`, `valueof=bytelen(A)+bytelen(B)`. The built-in `bytelen`/`count` take exactly one field-name argument. **Custom evaluators** (registered with `Marshaler.AddValueOf`) may take several — `valueof=CRC32(Type, Data)` — see [Custom valueof evaluators](#custom-valueof-evaluators-checksums-crcs) below. The option splitter is parenthesis-aware, so commas inside a function call's argument list do not split the tag's option list.

**Reference scope (forward references permitted).** Because the entire Go value is available at encode time, a `valueof` expression may reference **any** field in the struct, including fields declared *after* it. This is the deliberate counterpart to decode-side `[arrayLen]`/`buf_len` expressions, which remain **arithmetic-only** and may reference only **preceding** fields. Function tokens (`bytelen`/`count`) are rejected outside `valueof`.

**`bytelen()` evaluation.** Size is obtained by encoding `F` with the active Marshaler into a scratch buffer and counting the bytes — guaranteeing it equals what is actually written. (Raw `len()` is **not** used for strings, since text encodings such as Shift-JIS change the byte width.) Implementations may fast-path trivially-sized targets — byte slices, fixed-width scalars, and fixed arrays of scalars — with `len`/byte-width arithmetic to avoid a second encode.

**Validation (at `getStructMetadata`):** the target is an integer/bitmap kind; the argument names an existing field; for the built-ins, the function name is `bytelen` or `count` (one argument), and a `count` argument is a slice or array field; reference cycles among `valueof` fields are rejected. A non-built-in function name marks a custom evaluator (see below).

| Path | Mapping |
| :--- | :--- |
| **Runtime** (`marshal.go`, `unsafe_io.go`) | Before writing the field, if `valueofExpr` is set, evaluate it with the context-carrying value evaluator (Marshaler + byte order + struct metadata) and write the resulting integer in place of the field value. The unsafe path routes `valueof` fields through the reflection writer (rare fields; negligible cost). A custom evaluator is dispatched to its registered `ValueOfFunc` instead (`evalCustomValueof`), and is re-run on decode for validation (`validateCustomValueofs`, a post-decode pass in both `readStruct` and `unsafeReadStruct`). |
| **Codegen** (`generator.go`) | Emit the value computation inline before the integer write. `count(F)` → `len(s.F)`. `bytelen(F)` is resolved for nearly every field shape: byte slices/arrays and raw `string` (`len`), fixed `string(N)` buffers (the buffer width), all length-prefixed/null-terminated string variants (`prefix + content + terminator`), text-encoded strings (an `ms`-guarded `EncodeText` measurement matching the encode path), fixed-width scalars and scalar arrays (`width × count`), and nested structs / tag-counted arrays-of-structs / pointer-to-struct (a byte-exact runtime measurement via `binarystruct.Write(io.Discard, …)` that mirrors the encode path; a `nil` pointer contributes `0`). Still rejected at generation time (`bytelenExpr` returns an error → use the runtime interpreter): `bytelen` of a **pointer-element struct array** or a **pointer scalar field**, and a `valueof` expression that references another `valueof` field. A **custom evaluator** is supported when every referenced field is a *byte-region* field or a *fixed-width integer scalar* (see below); a transformed arg fails generation. |

#### Custom valueof evaluators (checksums, CRCs)

The built-in `bytelen`/`count` cover length and count fields. The other common
*derived* field in real formats is a **checksum / CRC** over preceding bytes,
which `bytelen` cannot express. Register a named evaluator on the Marshaler and
reference it from a `valueof=NAME(field, …)` tag:

```go
ms := binarystruct.NewMarshaler()
ms.AddValueOf("CRC32", func(c binarystruct.ValueOfContext) (uint64, error) {
    h := crc32.NewIEEE()
    for _, a := range c.Args { h.Write(a.Bytes) }
    return uint64(h.Sum32()), nil
})

type Chunk struct {
    _      struct{} `binary:"endian=big"`
    Length uint32   `binary:"uint32,valueof=bytelen(Data)"`
    Type   string   `binary:"string(4)"`
    Data   []byte   `binary:"[Length]byte"`
    CRC    uint32   `binary:"uint32,valueof=CRC32(Type, Data)"` // CRC over the two fields' encoded bytes
}
```

- **Registration is per-Marshaler** (like custom `Codec`s, not a package global):
  use a configured `Marshaler` — `ms.Marshal`/`ms.Unmarshal` — not the package-level
  functions. An unregistered name fails loud (`AddValueOf` / `GetValueOf` /
  `RemoveValueOf` manage the registry). The name must not be `bytelen` or `count`.
- **The handler receives `ValueOfContext`** with, for each referenced field, its
  **encoded bytes** (`Args[i].Bytes` — exactly what is written to / was read from
  the stream, honoring byte order and text encoding) and its Go value
  (`Args[i].Value`). **Compute checksums over `Bytes`, not `Value`** — byte order,
  text encoding, and field width change what actually hits the stream. `Target`
  names the field being computed; `Struct` is a pointer to the (de)serialized
  struct. It returns the integer to write (per the target field's binary type).
- **Encode + decode (unlike `bytelen`, which is encode-only).** On encode the
  evaluator produces the value written to the target field (the Go field is
  ignored — emit-only, no write-back, as above). On decode the *same* evaluator
  runs again (`ValueOfContext.Decoding == true`) over the decoded fields and the
  result is compared to the value read from the stream; a mismatch is reported as
  `DecodeError` wrapping `ErrValidationError`, naming the field. Validation runs
  as a **post-decode pass** over the whole struct, so a checksum may reference
  fields declared after it.
- **Grammar.** A custom evaluator must be the *entire* `valueof` expression —
  `valueof=CRC32(Type, Data)` — and cannot be combined with arithmetic or other
  functions in this version (e.g. `CRC32(Data)+1` is rejected at metadata time).
- **Memory/CPU.** Each referenced field's bytes are materialized on demand, so
  peak memory is bounded by the largest referenced field, not the struct or
  stream; the streaming `Write` path stays streaming. A **raw byte region** (byte
  slice/array at natural length, or an unencoded raw string) is handed back
  directly by `fieldEncodedBytes` (`rawByteRegionBytes`) — no re-encode — so a
  checksum/`bytelen` over `[]byte` allocates a constant amount regardless of
  payload size. Other shapes are measured with the same scratch-encode `bytelen`
  uses.
- **Codegen support.** The static generator emits a custom evaluator's argument
  bytes inline for **byte-region** fields (`[]byte`/`[N]byte` at natural length, a
  raw `string`, or a constant-size `string(N)` without text encoding) and
  **fixed-width integer scalars** (`int8`…`int64`, `uint8`…`uint64`,
  `byte`/`word`/`dword`/`qword`, via `order.PutUintN`) — no Marshaler call,
  honoring the runtime `order`. **Every other shape** (text-encoded or
  length-prefixed/terminated strings, floats, multibyte-scalar arrays, padded byte
  slices, variable string buffers) is re-encoded with its own tag via
  `ms.MarshalAs` — the runtime encoder, so the bytes match `fieldEncodedBytes`
  exactly. The **one** unsupported shape is a **nested-struct argument** (its byte
  order can't be expressed in a standalone tag): generation fails with a clear
  message → use the runtime interpreter for that struct.
  Generated encode requires a non-nil Marshaler (like `codec=`; the no-arg
  `MarshalBinary` passes `nil` and errors at run time — call
  `WriteBinaryWithMarshaler` with a Marshaler that has the evaluator registered).
  Decode-time validation is **on by default** — generated decode recomputes the
  evaluator and verifies it, matching the runtime interpreter. The `-no-validate`
  codegen flag strips **all** decode validation (this custom check plus the
  built-in `const`/`range`/`match` checks); with it set, the field is read as a
  plain scalar and not verified.

**Validation (at `getStructMetadata`):** a function name other than `bytelen`/`count`
marks a custom evaluator; the whole expression must then be a single such call,
its target an integer/bitmap kind, and its arguments existing fields. The
evaluator itself is looked up by name on the Marshaler at run time (metadata is
cached per type, evaluators are registered per Marshaler), so an unregistered or
misspelled name surfaces as a run-time error, not a parse-time one.

### Fixed / Magic Values: `const`

The `const` option pins a field to a fixed value: **emitted on encode** (the Go field is ignored, like `valueof`) and **validated on decode** (mismatch → `DecodeError` wrapping `ErrValidationError`). It serves format signatures, version markers, and reserved fields.

**Two target shapes.**
* **Integer/bitmap** (`const=0x04034b50`): a constant integer expression (the same arithmetic evaluator as size expressions, restricted to literals/operators — no field refs or functions). Stored as `constInt int64`; emitted as an integer, so the bytes follow the field's byte order — pair with `endian=` for a deterministic signature. Restricted to values `< 2^63`.
* **Byte sequence** (`[N]byte` / `[]byte` / `string(N)`, `const=0x89504e47…`): a hex blob decoded to `constBytes []byte` in natural order (endian-independent). The field's fixed size must equal `len(constBytes)`.

**Validation (at `getStructMetadata`, via `resolveConst`):** target is integer/bitmap or a raw byte sequence; `const` is not combined with `valueof`; the byte form is not combined with `encoding=` and has a fixed size matching the constant's length; the integer expression and hex blob parse cleanly.

| Path | Mapping |
| :--- | :--- |
| **Runtime** (`marshal.go`, `unsafe_io.go`) | On encode, substitute the field value with the constant (`synthIntValue`/`synthBytesValue`) and write normally — the unsafe path routes through the reflection writer like `valueof`. On decode, `validateField` → `validateConst` compares the read value (integer equality or `bytes.Equal`) and returns `ErrValidationError` on mismatch. |
| **Codegen** (`generator.go`) | Encode: integer → write the constant expression via the scalar writer (honors endian); byte sequence → `w.Write([]byte{…})`. Decode: read the field, then emit `if v != EXPR` (integer) or `if !bytes.Equal(v, []byte{…})` (bytes) returning a `*DecodeError` wrapping `ErrValidationError` (offset + field, matching the runtime). Both shapes supported. The `-no-validate` flag strips this decode check (along with `range`/`match` and custom-`valueof` validation). |

---

## 2. Dynamic vs. Static Path Architecture

> **Byte order is declared on the struct** — a blank `_ struct{}` field tagged
> `binary:"endian=…"` (parsed into `structMetadata.endian`), or inherited from a
> value-embedded struct that declares one (conflicting embedded orders are an
> error). The `Marshal`/`Unmarshal`/`Write`/`Read`/`Append`/`Inspect` package
> functions and methods take **no** `order` argument. Resolution, most specific
> first: per-field `endian=` → the struct's declaration → the `Marshaler.Order`
> fallback (`NewMarshalerOrder(order)`) → otherwise a multi-byte value fails loud.
> Each struct entry seeds `order = resolveByteOrder(order, meta.endian)` before the
> per-field resolution, identically across all three paths (the generated method
> seeds it too, since the runtime fast-paths into it before seeding). Codegen does
> not support struct-level `endian=inverse` or order-via-embedding.
>
> The same sentinel also carries **`encoding=`** (a default text encoding), parsed
> into `structMetadata.defaultEncoding` and baked into each string field's metadata
> that declares no `encoding=` of its own (so it sits between a per-field
> `encoding=` and `Marshaler.DefaultTextEncoding`). Codegen mirrors this — it bakes
> the struct-level encoding into each un-tagged string field's emitted
> `EncodeText`/`DecodeText`; only inheritance via embedding is codegen-unsupported.

```mermaid
graph TD
    A[Struct Definition with binary Tags] --> B{Execution Mode}
    B -->|Development / Dynamic| C[Runtime Interpreter]
    B -->|Production / Static| D[Codegen Compiler]
    
    C --> C1[Parser: struct.go]
    C1 --> C2[Safe Mode: unmarshal.go]
    C1 --> C3[Unsafe Mode: unsafe_io.go]
    
    D --> D1[AST Parser: binarystruct-codegen/generator.go]
    D1 --> D2[Go Writer: Generated Methods]
    
    C2 --> E[Encoded Stream]
    C3 --> E
    D2 --> E
```

### Fast-Path Interface Dispatch

When a struct is passed to `Write`/`Read`, the runtime checks for these interfaces in priority order before falling back to reflection-based field iteration:

| Priority | Interface | Defined In | Purpose |
| :--- | :--- | :--- | :--- |
| 1 | `MarshalerContextWriter` / `MarshalerContextReader` | `marshal.go` / `unmarshal.go` | Generated code needing Marshaler access (text encodings, custom codecs). |
| 2 | `BinaryWriter` / `BinaryReader` | `marshal.go` / `unmarshal.go` | Generated code with no runtime dependencies. |
| 3 | `encoding.BinaryMarshaler` / `encoding.BinaryUnmarshaler` | Go stdlib | Standard library compatibility fallback. |

### Alignment Constraints
1. **Expression Evaluation**:
   * **Runtime**: Resolves expressions dynamically at execution time using `evaluateTagValue` which interprets terms referencing field values.
   * **Codegen**: Emits raw Go code with the variables prefixed by `s.` (e.g. `s.PayloadSz - 2`), delegating calculation to the Go runtime.
2. **End-of-Stream Omission (`omittable`)**:
   * **Runtime**: Catches `io.EOF` / `io.ErrUnexpectedEOF` at field start and silently terminates decoding.
   * **Codegen**: Generates a peek check on `r` (reading 1 byte, checking for EOF, and restoring via `io.MultiReader`) before reading the field.
3. **Regex Precompilation (`match=pattern`)**:
   * **Runtime**: Compiles regex once during `getStructMetadata()` via `regexp.Compile()` and stores it in the metadata cache.
   * **Codegen**: Declares a global package-level variable `var regex_Struct_Field = regexp.MustCompile(pattern)` to precompile at package load.
4. **Computed Assignment (`valueof`) & Expression Functions**:
   * **Runtime**: `valueof` fields are resolved at encode time only, via a context-carrying evaluator able to compute `bytelen()`/`count()`; the result is written without mutating the struct (emit-only). `bytelen()` measures into a scratch buffer, so the output stream stays forward-only. Functions are `valueof`-only; decode expressions remain arithmetic-only and may reference preceding fields only.
   * **Codegen**: Emits the value computation inline before the field write — `len(s.F)` for `count`, and for `bytelen` resolves nearly every field shape (scalars and scalar arrays, byte slices/arrays, all string variants including text-encoded, nested structs, tag-counted arrays of structs, and pointer-to-struct; see §1's `valueof` codegen row for the exact mapping). It still rejects `bytelen` of a pointer-element struct array or a pointer scalar field at generation time (use the runtime interpreter). No write-back is generated.

---

## 3. Extension Protocol (Sync Checklist)

When introducing a new binary type, tag option, or modifier, you **must** check off all of the following:

> **Struct-scope options** (carried on the blank `_ struct{}` sentinel, e.g.
> `endian=`/`encoding=`) need extra care beyond a normal field option: parse them
> in `parseStructSentinel` and store on `structMetadata` (Step 2); seed/apply them
> at every struct entry in all three paths (Step 3/4); and in codegen either
> support them in `generateMethods` or **fail loud** (Step 5) — never emit code
> that silently ignores the option. Mirror the resolution precedence and the
> non-`struct{}` sentinel guard.

- [ ] **Step 1: Syntax Spec**
  Add the syntax definition to `STRUCT_TAGS.md` and `STRUCT_TAGS_ja.md`. Update Section 1 of this document (`SPECIFICATION.md`).

- [ ] **Step 2: Struct Analysis (`struct.go`)**
  Update `structFieldMetadata` to store parsed parameters. Update the tag-parsing logic in `parseTagString` to parse the new option/type.

- [ ] **Step 3: Safe-Mode Interpreter (`marshal.go` / `unmarshal.go`)**
  Update `readMain` / `writeMain` (and scalar/slice handlers) to implement the runtime behavior using standard reflection.

- [ ] **Step 4: Unsafe-Mode Interpreter (`unsafe_io.go`)**
  Update the pointer-offset loop in `unsafeWriteStruct` and `unsafeReadStruct` to optimize operations using direct memory access (unsafe pointer casting).

- [ ] **Step 5: Code Generator (`binarystruct-codegen/generator.go`)**
  Update `parseFieldTag` to extract the option/type. Update `generateFieldWrite` / `generateFieldRead` (and array handlers) to emit the compiled Go statements.

- [ ] **Step 6: Interface Verifications**
  If the option accesses dynamic instances (like text encodings or custom codecs), ensure it is wired through both the standard interface and context-aware interfaces (`MarshalerContextWriter` / `MarshalerContextReader`).

- [ ] **Step 7: Testing**
  Write unit tests verifying correct behavior in both safe and unsafe interpreter modes, and add a test structure verifying the same behavior in the static codegen integration suite ([codegen_integration_test.go](codegen_integration_test.go)).
