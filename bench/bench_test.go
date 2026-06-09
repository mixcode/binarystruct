// Copyright 2026 github.com/mixcode

package bench

import (
	"bytes"
	"testing"

	bst "github.com/mixcode/binarystruct"
)

// One reused little-endian Marshaler for the runtime benchmarks (construction is
// amortized in real use; we measure the encode/decode path, not allocation of the
// Marshaler). The selected runtime engine — unsafe (default) or safe
// (`-tags safe_binarystruct`) — is chosen at compile time.
var ms = bst.NewMarshalerOrder(bst.LittleEndian)

func mkHeader() Header {
	return Header{A: 1, B: -2, C: 3, D: -4, E: 5, F: -6, G: 7, H: -8, I: 9.5, J: -10.25}
}
func mkIntSlice() IntSlice {
	d := make([]uint32, 1024)
	for i := range d {
		d[i] = uint32(i * 2654435761)
	}
	return IntSlice{Data: d}
}
func mkRecord() Record {
	p := make([]byte, 256)
	for i := range p {
		p[i] = byte(i)
	}
	return Record{Name: []byte("benchmark-record"), Seq: 0xdeadbeef, Flags: 0x1234, Payload: p}
}
func mkNested() Nested {
	it := make([]Inner, 64)
	for i := range it {
		it[i] = Inner{X: uint32(i), Y: uint16(i), Z: byte(i)}
	}
	return Nested{Items: it}
}

// blobOf encodes via the (mode-selected) runtime, for the Unmarshal benchmarks.
func blobOf(v interface{}) []byte {
	b, err := ms.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// --- Header ---------------------------------------------------------------
func BenchmarkRuntime_Header_Marshal(b *testing.B) {
	in := mkHeader()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ms.Marshal(&in); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_Header_Marshal(b *testing.B) {
	in := mkHeader()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := in.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkRuntime_Header_Unmarshal(b *testing.B) {
	blob := blobOf(mkHeader())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out Header
		if _, err := ms.Unmarshal(blob, &out); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_Header_Unmarshal(b *testing.B) {
	blob := blobOf(mkHeader())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out Header
		if err := out.UnmarshalBinary(blob); err != nil {
			b.Fatal(err)
		}
	}
}

// --- IntSlice -------------------------------------------------------------
func BenchmarkRuntime_IntSlice_Marshal(b *testing.B) {
	in := mkIntSlice()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ms.Marshal(&in); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_IntSlice_Marshal(b *testing.B) {
	in := mkIntSlice()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := in.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkRuntime_IntSlice_Unmarshal(b *testing.B) {
	blob := blobOf(mkIntSlice())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out IntSlice
		if _, err := ms.Unmarshal(blob, &out); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_IntSlice_Unmarshal(b *testing.B) {
	blob := blobOf(mkIntSlice())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out IntSlice
		if err := out.UnmarshalBinary(blob); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Record ---------------------------------------------------------------
func BenchmarkRuntime_Record_Marshal(b *testing.B) {
	in := mkRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ms.Marshal(&in); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_Record_Marshal(b *testing.B) {
	in := mkRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := in.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkRuntime_Record_Unmarshal(b *testing.B) {
	blob := blobOf(mkRecord())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out Record
		if _, err := ms.Unmarshal(blob, &out); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_Record_Unmarshal(b *testing.B) {
	blob := blobOf(mkRecord())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out Record
		if err := out.UnmarshalBinary(blob); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Nested ---------------------------------------------------------------
func BenchmarkRuntime_Nested_Marshal(b *testing.B) {
	in := mkNested()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ms.Marshal(&in); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_Nested_Marshal(b *testing.B) {
	in := mkNested()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := in.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkRuntime_Nested_Unmarshal(b *testing.B) {
	blob := blobOf(mkNested())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out Nested
		if _, err := ms.Unmarshal(blob, &out); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkCodegen_Nested_Unmarshal(b *testing.B) {
	blob := blobOf(mkNested())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out Nested
		if err := out.UnmarshalBinary(blob); err != nil {
			b.Fatal(err)
		}
	}
}

// TestBenchParity keeps the suite honest: for every workload the codegen output
// must be byte-identical to the runtime, and a codegen decode+re-encode must
// reproduce the bytes (round-trip; comparing bytes sidesteps emit-only valueof
// length fields). Runs under `go test` in whichever mode is built — also a
// bitrot guard for the benchmark structs.
func TestBenchParity(t *testing.T) {
	assertParity := func(name string, in interface{}, gen []byte, genErr error, roundtrip func([]byte) ([]byte, error)) {
		rt, err := ms.Marshal(in)
		if err != nil {
			t.Fatalf("%s runtime marshal: %v", name, err)
		}
		if genErr != nil {
			t.Fatalf("%s codegen marshal: %v", name, genErr)
		}
		if !bytes.Equal(rt, gen) {
			t.Errorf("%s: codegen %x != runtime %x", name, gen, rt)
		}
		rt2, err := roundtrip(rt)
		if err != nil {
			t.Fatalf("%s codegen round-trip: %v", name, err)
		}
		if !bytes.Equal(rt, rt2) {
			t.Errorf("%s: codegen decode+re-encode differs from the original bytes", name)
		}
	}

	h := mkHeader()
	hg, he := h.MarshalBinary()
	assertParity("Header", &h, hg, he, func(b []byte) ([]byte, error) {
		var o Header
		if err := o.UnmarshalBinary(b); err != nil {
			return nil, err
		}
		return o.MarshalBinary()
	})

	is := mkIntSlice()
	ig, ie := is.MarshalBinary()
	assertParity("IntSlice", &is, ig, ie, func(b []byte) ([]byte, error) {
		var o IntSlice
		if err := o.UnmarshalBinary(b); err != nil {
			return nil, err
		}
		return o.MarshalBinary()
	})

	r := mkRecord()
	rg, re := r.MarshalBinary()
	assertParity("Record", &r, rg, re, func(b []byte) ([]byte, error) {
		var o Record
		if err := o.UnmarshalBinary(b); err != nil {
			return nil, err
		}
		return o.MarshalBinary()
	})

	n := mkNested()
	ng, ne := n.MarshalBinary()
	assertParity("Nested", &n, ng, ne, func(b []byte) ([]byte, error) {
		var o Nested
		if err := o.UnmarshalBinary(b); err != nil {
			return nil, err
		}
		return o.MarshalBinary()
	})
}
