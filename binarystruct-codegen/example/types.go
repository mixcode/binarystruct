// Copyright 2026 github.com/mixcode

//go:generate go run github.com/mixcode/binarystruct/binarystruct-codegen -type Packet,Chunk
package example

// Packet shows the basics. It declares its byte order on the struct itself (the
// blank `_ struct{}` sentinel), so the generator needs no -endian flag. Magic is a
// const signature — emitted automatically on encode (the Go field is ignored) and
// validated on decode — and Version is range-checked on decode.
type Packet struct {
	_       struct{} `binary:"endian=big"`               // struct-level byte order
	Magic   [4]byte  `binary:"[4]byte,const=0x50414b31"` // "PAK1", emit-only + validated on decode
	Seq     uint32   `binary:"uint32"`
	Version uint8    `binary:"uint8,range=1..10"` // validated on decode
	Payload []byte   `binary:"[8]byte"`           // 8-byte fixed array
}

// Chunk shows the custom valueof workflow with a PNG-chunk-style record. Length is
// the built-in bytelen() evaluator (the encoded byte length of Data). CRC is a
// custom "CRC32" evaluator computed over the encoded bytes of Type+Data; it is
// registered on a Marshaler at run time, so encode/decode go through the
// *WithMarshaler methods. The CRC is written on encode and re-checked on decode
// (validation is on by default; pass -no-validate to skip it).
type Chunk struct {
	_      struct{} `binary:"endian=big"`
	Length uint32   `binary:"uint32,valueof=bytelen(Data)"`
	Type   string   `binary:"string(4)"`
	Data   []byte   `binary:"[Length]byte"`
	CRC    uint32   `binary:"uint32,valueof=CRC32(Type, Data)"`
}
