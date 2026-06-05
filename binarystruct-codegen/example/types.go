// Copyright 2026 github.com/mixcode

//go:generate go run github.com/mixcode/binarystruct/binarystruct-codegen -type Packet -endian big -output packet_binary.go
package example

type Packet struct {
	Magic   string `binary:"[4]byte"` // 4-byte string
	Seq     uint32 `binary:"uint32"`  // Big-endian uint32
	Version uint8  `binary:"uint8,range=1..10"`
	Payload []byte `binary:"[8]byte"` // 8-byte fixed array
}
