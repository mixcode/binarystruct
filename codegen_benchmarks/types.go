// Copyright 2026 github.com/mixcode

package codegen_benchmarks

type BenchSubHeader struct {
	Type     uint16  `binary:"uint16"`
	Sequence uint32  `binary:"uint32"`
	Checksum float64 `binary:"float64"`
	Param1   uint8   `binary:"uint8"`
	Param2   uint8   `binary:"uint8"`
	Param3   uint16  `binary:"uint16"`
	Param4   uint32  `binary:"uint32"`
	Param5   int32   `binary:"int32"`
}

type BenchPacket struct {
	Magic     uint32         `binary:"uint32"`
	Version   uint8          `binary:"uint8,range=1..5"`
	Flags     uint8          `binary:"uint8"`
	PayloadSz uint16         `binary:"uint16"`
	Id        string         `binary:"string(8)"`
	Buffer    []byte         `binary:"[PayloadSz]byte"`
	Nested    BenchSubHeader `binary:"any"`
}

type BenchSubHeaderDynamic struct {
	Type     uint16  `binary:"uint16"`
	Sequence uint32  `binary:"uint32"`
	Checksum float64 `binary:"float64"`
	Param1   uint8   `binary:"uint8"`
	Param2   uint8   `binary:"uint8"`
	Param3   uint16  `binary:"uint16"`
	Param4   uint32  `binary:"uint32"`
	Param5   int32   `binary:"int32"`
}

type BenchPacketDynamic struct {
	Magic     uint32                `binary:"uint32"`
	Version   uint8                 `binary:"uint8,range=1..5"`
	Flags     uint8                 `binary:"uint8"`
	PayloadSz uint16                `binary:"uint16"`
	Id        string                `binary:"string(8)"`
	Buffer    []byte                `binary:"[PayloadSz]byte"`
	Nested    BenchSubHeaderDynamic `binary:"any"`
}
