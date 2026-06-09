// Copyright 2026 github.com/mixcode

//go:generate go run github.com/mixcode/binarystruct/binarystruct-codegen -type Header,IntSlice,Record,Inner,Nested -endian little

// Package bench holds the cross-mode performance benchmark suite (safe runtime vs
// unsafe runtime vs static codegen) over a few representative wire shapes. The
// generated *_binary.go files are committed; regenerate with `go generate ./bench`.
// Run the whole comparison and refresh the README table with `make bench`.
package bench

// Header: a flat record of fixed-width scalars (the per-field path).
type Header struct {
	_ struct{} `binary:"endian=little"`
	A uint8    `binary:"uint8"`
	B int8     `binary:"int8"`
	C uint16   `binary:"uint16"`
	D int16    `binary:"int16"`
	E uint32   `binary:"uint32"`
	F int32    `binary:"int32"`
	G uint64   `binary:"uint64"`
	H int64    `binary:"int64"`
	I float32  `binary:"float32"`
	J float64  `binary:"float64"`
}

// IntSlice: a length-prefixed slice of multibyte scalars (the bulk-buffer path).
type IntSlice struct {
	_    struct{} `binary:"endian=little"`
	N    uint32   `binary:"uint32,valueof=count(Data)"`
	Data []uint32 `binary:"[N]uint32"`
}

// Record: a realistic variable-length record — const magic, an auto-length name,
// some scalars, and an auto-length payload.
type Record struct {
	_       struct{} `binary:"endian=little"`
	Magic   [4]byte  `binary:"[4]byte,const=0x4d594252"`
	NameLen uint16   `binary:"uint16,valueof=bytelen(Name)"`
	Name    []byte   `binary:"[NameLen]byte"`
	Seq     uint32   `binary:"uint32"`
	Flags   uint16   `binary:"uint16"`
	PayLen  uint32   `binary:"uint32,valueof=bytelen(Payload)"`
	Payload []byte   `binary:"[PayLen]byte"`
}

// Inner / Nested: a slice of a nested generated struct (the nested direct-call path).
type Inner struct {
	_ struct{} `binary:"endian=little"`
	X uint32   `binary:"uint32"`
	Y uint16   `binary:"uint16"`
	Z uint8    `binary:"uint8"`
}
type Nested struct {
	_     struct{} `binary:"endian=little"`
	Count uint16   `binary:"uint16,valueof=count(Items)"`
	Items []Inner  `binary:"[Count]any"`
}
