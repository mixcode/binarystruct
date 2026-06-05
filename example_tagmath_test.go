// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"
)

type DynamicPacket struct {
	HeaderSize uint8
	PayloadLen uint16
	// The buffer size is dynamically calculated at runtime using sibling fields
	Data []byte `binary:"[(HeaderSize*2) + PayloadLen]byte"`
}

func Example_tagMath() {
	in := DynamicPacket{
		HeaderSize: 4,
		PayloadLen: 6,
		Data:       []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e},
	}

	// Marshal structural data
	blob, err := binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Marshal(in)
	if err != nil {
		panic(err)
	}

	// Calculated size: (4*2) + 6 = 14 bytes for Data.
	// Total blob: 1 (HeaderSize) + 2 (PayloadLen) + 14 (Data) = 17 bytes.
	fmt.Printf("Blob size: %d bytes\n", len(blob))
	fmt.Printf("Blob: %x\n", blob)

	var restored DynamicPacket
	_, err = binarystruct.NewMarshalerOrder(binarystruct.BigEndian).Unmarshal(blob, &restored)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Restored PayloadLen: %d, Data size: %d\n", restored.PayloadLen, len(restored.Data))

	// Output:
	// Blob size: 17 bytes
	// Blob: 0400060102030405060708090a0b0c0d0e
	// Restored PayloadLen: 6, Data size: 14
}
