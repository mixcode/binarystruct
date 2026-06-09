// Copyright 2026 github.com/mixcode

package example

import "testing"

func benchSamples() Samples {
	v := make([]uint32, 1000)
	for i := range v {
		v[i] = uint32(i)
	}
	return Samples{V: v}
}

func BenchmarkCodegenScalarSliceMarshal(b *testing.B) {
	in := benchSamples()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := in.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCodegenScalarSliceUnmarshal(b *testing.B) {
	in := benchSamples()
	blob, _ := in.MarshalBinary()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out Samples
		if err := out.UnmarshalBinary(blob); err != nil {
			b.Fatal(err)
		}
	}
}

func benchRec() Rec {
	items := make([]Item, 50)
	for i := range items {
		items[i] = Item{A: uint32(i), B: uint16(i)}
	}
	return Rec{Items: items}
}

func BenchmarkCodegenNestedMarshal(b *testing.B) {
	in := benchRec()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := in.MarshalBinary(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCodegenNestedUnmarshal(b *testing.B) {
	in := benchRec()
	blob, _ := in.MarshalBinary()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out Rec
		if err := out.UnmarshalBinary(blob); err != nil {
			b.Fatal(err)
		}
	}
}
