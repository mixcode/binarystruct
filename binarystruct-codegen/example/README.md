# Codegen Example

This directory contains two self-contained struct layouts demonstrating how to use `binarystruct-codegen`. [`types.go`](types.go) defines both; [`example_test.go`](example_test.go) round-trips each and shows the failure modes.

- **`Packet`** — the basics. It declares its byte order on the struct itself (the
  blank `_ struct{}` sentinel tagged `endian=big`), so the `go:generate` directive
  needs **no `-endian` flag**. It carries a `const` magic signature (emitted on
  encode, validated on decode) and a `range=1..10`-checked field; the test asserts
  that an out-of-range value is rejected with a `*binarystruct.DecodeError`.
- **`Chunk`** — a PNG-chunk-style record showing the **custom `valueof`** workflow.
  `Length` uses the built-in `bytelen(Data)`; `CRC` uses a custom `CRC32(Type, Data)`
  evaluator registered on a `Marshaler` with `AddValueOf`. Because that needs the
  Marshaler, the test drives the generated `WriteBinaryWithMarshaler` /
  `ReadBinaryWithMarshaler` methods. The CRC is computed on encode and re-validated
  on decode (on by default — pass `-no-validate` to skip), so a corrupted CRC is
  rejected with a `*DecodeError`.

## Running the Example

`packet_binary.go` is committed for reference, but it is generated — re-run the
generator before the tests (it overwrites the file in place):

```bash
# Generate the serialization methods (packet_binary.go)
go generate ./...

# Run the example tests
go test ./...
```

> The `go:generate` directive runs the generator via `go run …/binarystruct-codegen`.
> Because this example lives inside the `binarystruct-codegen` module, that builds
> and runs the generator from the local source — no separately installed binary.
