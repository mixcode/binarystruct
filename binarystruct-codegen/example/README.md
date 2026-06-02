# Codegen Example

This directory contains a simple, self-contained struct layout demonstrating how to use `binarystruct-codegen`.

## Running the Example

Before running the tests, you must trigger the code generator:

```bash
# Generate the serialization methods (packet_binary.go)
go generate ./...

# Run the example tests
go test ./...
```
