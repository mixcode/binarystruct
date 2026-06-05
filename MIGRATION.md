# Migrating to v0.3.0

v0.3.0 is a breaking cleanup that aligns `binarystruct` with Go standard-library
conventions and moves byte order onto the struct. This guide lists every change
and its mechanical fix.

> **AI agents:** the full usage manual is [llms-full.txt](llms-full.txt); this
> file is the upgrade path from 0.2.x.

## At a glance

| Area | Before (0.2.x) | After (0.3.0) |
| :--- | :--- | :--- |
| Type name | `binarystruct.Marshaller` | `binarystruct.Marshaler` |
| Byte order | `Marshal(v, order)` / `Unmarshal(data, order, v)` | declare on the struct, then `Marshal(v)` / `Unmarshal(data, v)` |
| Constructor | `NewMarshaler(order)` | `NewMarshalerOrder(order)` (or `NewMarshaler()` for none) |
| Custom hook type | `Serializer` | `Codec` |
| Hook methods | `Serialize` / `Deserialize` | `Encode` / `Decode` |
| Hook registration | `AddSerializer` / `RemoveSerializer` / `GetSerializer` | `AddCodec` / `RemoveCodec` / `GetCodec` |
| Custom-hook tag | `serializer=NAME` | `codec=NAME` |
| Codegen order | (baked big-endian) | `-endian` flag (optional if the struct declares one) |
| Minimum Go | 1.21 | 1.24 |

## 1. Spelling: `Marshaller` → `Marshaler`

A pure rename (Go's single-`l` stdlib spelling), including the generated
`WriteBinaryWithMarshaler` / `ReadBinaryWithMarshaler` methods.

```go
var ms binarystruct.Marshaller   // before
var ms binarystruct.Marshaler    // after
```

For a gradual migration of *type references* only, you can add a local alias —
but note it does **not** fix the API changes below:

```go
type Marshaller = binarystruct.Marshaler // shim: spelling only
```

The package ships no deprecated alias.

## 2. Byte order: declare it on the struct (the big one)

Byte order is no longer passed at the call site. A struct declares its order once
with a blank `_ struct{}` sentinel field, and the whole API is order-free.

```go
// before
type Header struct {
	Magic uint32 `binary:"uint32"`
	Size  uint32 `binary:"uint32"`
}
blob, _ := binarystruct.Marshal(&h, binarystruct.BigEndian)
_, _   = binarystruct.Unmarshal(blob, binarystruct.BigEndian, &h)

// after
type Header struct {
	_     struct{} `binary:"endian=big"` // declare the order once, on the type
	Magic uint32   `binary:"uint32"`
	Size  uint32   `binary:"uint32"`
}
blob, _ := binarystruct.Marshal(&h)        // no order argument
_, _   = binarystruct.Unmarshal(blob, &h)  // no order argument
```

- **Resolution order** (most specific first): a per-field `endian=` tag → the
  struct's `_` declaration → the `Marshaler.Order` fallback → otherwise encoding
  or decoding a multi-byte value **fails loud** (`"no byte order: declare endian=
  on the struct or use NewMarshalerOrder(order)"`).
- **Embedding** a struct that declares an order propagates it (a reusable base):

  ```go
  type bigEndian struct{ _ struct{} `binary:"endian=big"` } // 0 bytes; declares the order
  type PNG struct {
  	bigEndian
  	Width uint32 `binary:"uint32"`
  }
  ```
- **Values that can't declare an order** (a bare scalar, a third-party struct)
  take a fallback from `NewMarshalerOrder(order)`:

```go
ms := binarystruct.NewMarshalerOrder(binarystruct.BigEndian)
b, _ := ms.Marshal(&someScalar)
```

`NewMarshaler(order)` is replaced by `NewMarshalerOrder(order)`; plain
`NewMarshaler()` supplies no fallback (every value must declare its own order).
The `Marshal` / `Unmarshal` / `Write` / `Read` / `Append` / `Inspect` **methods**
also drop their `order` argument.

## 3. Custom codecs: `Serializer` → `Codec`

```go
// before
type MyCodec struct{}
func (MyCodec) Serialize(w io.Writer, v any, parent reflect.Value, i int, order binarystruct.ByteOrder) (int, error) { ... }
func (MyCodec) Deserialize(r io.Reader, parent reflect.Value, i int, order binarystruct.ByteOrder) (any, int, error) { ... }
ms.AddSerializer("my", MyCodec{})
// tag: `binary:"custom,serializer=my"`

// after
func (MyCodec) Encode(w io.Writer, v any, parent reflect.Value, i int, order binarystruct.ByteOrder) (int, error) { ... }
func (MyCodec) Decode(r io.Reader, parent reflect.Value, i int, order binarystruct.ByteOrder) (any, int, error) { ... }
ms.AddCodec("my", MyCodec{})
// tag: `binary:"custom,codec=my"`
```

(`RemoveSerializer`/`GetSerializer` → `RemoveCodec`/`GetCodec`.) The codec
methods keep their `order` parameter — it is the contextual order passed during
encoding/decoding, not a call-site argument.

## 4. Code generation (`binarystruct-codegen`)

- `-endian big|little` is now a flag. It is **optional** when the struct declares
  its own order (the `_` sentinel wins); otherwise it is required. Generation
  **errors** if neither the struct nor the flag provides an order.
- The generator now also emits `AppendBinary` (implementing
  `encoding.BinaryAppender`, Go 1.24).
- Update your `//go:generate` directives: `binarystruct-codegen -type T` →
  `binarystruct-codegen -type T -endian big` (or declare `endian=` on `T`).
- Not supported by codegen: struct-level `endian=inverse`, and order inheritance
  via embedding — use the runtime interpreter for those.

## 5. New: `Append` / `AppendBinary`

`binarystruct.Append(buf, v)` (and `Marshaler.Append(buf, v)`) encode a value and
append it to a buffer — the `encoding/binary.Append` analog. Generated types and
hand-written types can implement `encoding.BinaryAppender` via `AppendBinary`.

## 6. Gotchas

- **Recursion trap.** If you make a type implement `encoding.BinaryMarshaler` by
  delegating to `binarystruct.Marshal(self)`, it recurses forever — binarystruct
  honors `encoding.BinaryMarshaler`. Marshal a **method-less twin type** instead
  (see llms-full.txt §9 and the runnable example).
- **Go 1.24** is now the minimum (for `encoding.BinaryAppender`); bump your
  `go.mod` `go` directive.

## Quick checklist

- [ ] `Marshaller` → `Marshaler` (type), `…WithMarshaller` → `…WithMarshaler`.
- [ ] `Serializer`/`Serialize`/`Deserialize` → `Codec`/`Encode`/`Decode`;
      `Add/Remove/GetSerializer` → `…Codec`; tag `serializer=` → `codec=`.
- [ ] Declare byte order on each top-level struct (`_ struct{}` `binary:"endian=…"`)
      and drop the `order` argument from `Marshal`/`Unmarshal`/`Write`/`Read`/`Inspect`.
- [ ] For values that don't declare an order, use `NewMarshalerOrder(order)`;
      `NewMarshaler(order)` → `NewMarshalerOrder(order)`.
- [ ] Codegen: add `-endian` (or declare `endian=` on the struct) to each
      `//go:generate` directive.
- [ ] Set `go 1.24` (or later) in `go.mod`.
