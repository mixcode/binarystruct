// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"
)

func ExampleInspect() {
	type Packet struct {
		Magic      uint8
		PayloadLen uint16 `binary:"uint16,endian=little"`
		Data       []byte `binary:"[PayloadLen]byte"`
		Checksum   uint8  `binary:"uint8,omittable"`
	}

	pkt := Packet{
		Magic:      0xab,
		PayloadLen: 4,
		Data:       []byte{0x10, 0x20, 0x30, 0x40},
		Checksum:   0x99,
	}

	// Inspect the layout of the struct
	layout, err := binarystruct.Inspect(pkt, binarystruct.BigEndian)
	if err != nil {
		panic(err)
	}

	// Format with hex offsets/values and decimal sizes
	cfg := binarystruct.LayoutFormat{
		OffsetBase: 16,
		SizeBase:   10,
		ValueBase:  16,
	}
	table := layout.Format(cfg)
	fmt.Println(table)

	// Output:
	// Struct Layout: Packet (Total Size: 8)
	// ================================================================================================================
	// OFFSET   SIZE   FIELD NAME   GO TYPE   BINARY TYPE   ENDIAN         VALUE                   DETAILS
	// ----------------------------------------------------------------------------------------------------------------
	// 0x0      1      Magic        uint8     Uint8         BigEndian      0xab
	// 0x1      2      PayloadLen   uint16    Uint16        LittleEndian   0x4
	// 0x3      4      Data         []uint8   Byte          BigEndian      [0x10 0x20 0x30 0x40]   expr: PayloadLen
	// 0x7      1      Checksum     uint8     Uint8         BigEndian      0x99
	// ================================================================================================================
}
