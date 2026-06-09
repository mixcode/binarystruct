// Copyright 2026 github.com/mixcode

//go:generate go run github.com/mixcode/binarystruct/binarystruct-codegen -type Samples,Rec,Item -output samples_binary.go
package example

// Samples is a benchmark fixture for the codegen bulk-buffer scalar-slice path: a
// length-prefixed slice of multibyte scalars (the shape that previously generated
// a per-element write/read loop).
type Samples struct {
	_ struct{} `binary:"endian=little"`
	N uint32   `binary:"uint32,valueof=count(V)"`
	V []uint32 `binary:"[N]uint32"`
}

// Item / Rec are a benchmark fixture for the nested-struct path: a length-prefixed
// slice of a nested generated struct. Because Item is also generated, Rec calls
// Item's generated methods directly (no per-element runtime Marshaler).
type Item struct {
	_ struct{} `binary:"endian=little"`
	A uint32   `binary:"uint32"`
	B uint16   `binary:"uint16"`
}
type Rec struct {
	_     struct{} `binary:"endian=little"`
	N     uint16   `binary:"uint16,valueof=count(Items)"`
	Items []Item   `binary:"[N]any"`
}
