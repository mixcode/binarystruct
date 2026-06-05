// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"
)

type PaddedPacket struct {
	ID uint16
	// 2 bytes of padding to align the next field to a 4-byte boundary
	Padding1 interface{} `binary:"pad(2)"`
	Value    uint32
	// A slice of padding bytes, e.g. 4 bytes
	Padding2 interface{} `binary:"[4]pad"`
	Checksum uint8
	// An ignored field that won't be serialized or deserialized
	LocalOnly string `binary:"-"`
}

func Example_padding() {
	in := PaddedPacket{
		ID:        0x1234,
		Value:     0xabcdef00,
		Checksum:  0x99,
		LocalOnly: "temporary session data",
	}

	// Marshal structural data
	blob, err := binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Marshal(in)
	if err != nil {
		panic(err)
	}

	// Expected size:
	// ID (2) + Padding1 (2) + Value (4) + Padding2 (4) + Checksum (1) = 13 bytes
	// LocalOnly is ignored and doesn't write anything.
	fmt.Printf("Blob size: %d bytes\n", len(blob))
	fmt.Printf("Blob hex: %x\n", blob)

	var restored PaddedPacket
	restored.LocalOnly = "preserved value"
	_, err = binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Unmarshal(blob, &restored)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Restored ID: 0x%x, Value: 0x%x, Checksum: 0x%x\n", restored.ID, restored.Value, restored.Checksum)
	// LocalOnly should remain "preserved value" because unmarshal ignores it
	fmt.Printf("Restored LocalOnly: %q\n", restored.LocalOnly)

	// Output:
	// Blob size: 13 bytes
	// Blob hex: 12340000abcdef000000000099
	// Restored ID: 0x1234, Value: 0xabcdef00, Checksum: 0x99
	// Restored LocalOnly: "preserved value"
}
