// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"
)

type SubHeader struct {
	Magic   uint16
	Version uint16
}

type MainHeader struct {
	HeaderSize uint16
	// Nested struct. The active byte order propagates down.
	SubInfo SubHeader
	// Explicitly overridden endian for nested struct.
	// Its fields will inherit this overridden byte order.
	SubLeInfo SubHeader `binary:"any,endian=little"`
}

func Example_nestedStructs() {
	in := MainHeader{
		HeaderSize: 8,
		SubInfo: SubHeader{
			Magic:   0xabcd,
			Version: 0x0001,
		},
		SubLeInfo: SubHeader{
			Magic:   0xabcd,
			Version: 0x0001,
		},
	}

	// Marshal with BigEndian
	blob, err := binarystruct.Marshal(in, binarystruct.BigEndian)
	if err != nil {
		panic(err)
	}

	// Expected:
	// HeaderSize (BigEndian: 00 08)
	// SubInfo.Magic (BigEndian: ab cd)
	// SubInfo.Version (BigEndian: 00 01)
	// SubLeInfo.Magic (LittleEndian override: cd ab)
	// SubLeInfo.Version (LittleEndian override: 01 00)
	// Total: 2 + 2 + 2 + 2 + 2 = 10 bytes
	fmt.Printf("Blob hex: %x\n", blob)

	var restored MainHeader
	_, err = binarystruct.Unmarshal(blob, binarystruct.BigEndian, &restored)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Restored SubInfo: Magic=0x%x, Version=0x%x\n", restored.SubInfo.Magic, restored.SubInfo.Version)
	fmt.Printf("Restored SubLeInfo: Magic=0x%x, Version=0x%x\n", restored.SubLeInfo.Magic, restored.SubLeInfo.Version)

	// Output:
	// Blob hex: 0008abcd0001cdab0100
	// Restored SubInfo: Magic=0xabcd, Version=0x1
	// Restored SubLeInfo: Magic=0xabcd, Version=0x1
}
