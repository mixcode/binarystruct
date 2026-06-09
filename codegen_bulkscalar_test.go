// Copyright 2026 github.com/mixcode

package binarystruct_test

import "testing"

// TestCodegen_BulkScalarSlice_Parity exercises the bulk-buffer codegen path for
// fixed-width multibyte scalar arrays/slices (uint32 slice with a field-ref
// length, a fixed [3]int16 array, and a float32 slice). It asserts the generated
// MarshalBinary output is byte-identical to the runtime interpreter (so the bulk
// path preserves layout and endianness) and that the round trip recovers the value.
func TestCodegen_BulkScalarSlice_Parity(t *testing.T) {
	typesSrc := "type Arrays struct {\n" +
		"\t_ struct{}  `binary:\"endian=big\"`\n" +
		"\tN uint16    `binary:\"uint16,valueof=count(A)\"`\n" +
		"\tA []uint32  `binary:\"[N]uint32\"`\n" +
		"\tB [3]int16  `binary:\"[3]int16\"`\n" +
		"\tM uint16    `binary:\"uint16,valueof=count(C)\"`\n" +
		"\tC []float32 `binary:\"[M]float32\"`\n}\n"
	testSrc := "import (\n\t\"bytes\"\n\t\"reflect\"\n\t\"testing\"\n\n\t\"github.com/mixcode/binarystruct\"\n)\n\n" +
		"func TestBulk(t *testing.T) {\n" +
		"\tin := Arrays{A: []uint32{1, 0x01020304, 0xffffffff}, B: [3]int16{-1, 2, -300}, C: []float32{1.5, -2.25, 1024}}\n" +
		"\tgen, err := in.MarshalBinary()\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\trt, err := binarystruct.Marshal(&in) // runtime interpreter\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif !bytes.Equal(gen, rt) {\n\t\tt.Fatalf(\"codegen %x vs runtime %x\", gen, rt)\n\t}\n" +
		"\t// Spot-check big-endian layout: A[1] = 0x01020304 begins after N(2)+ (3*4 bytes start at offset 2).\n" +
		"\tif gen[2] != 0x00 || gen[6] != 0x01 || gen[7] != 0x02 {\n\t\tt.Fatalf(\"unexpected big-endian bytes: % x\", gen[:12])\n\t}\n" +
		"\tvar out Arrays\n" +
		"\tif err := out.UnmarshalBinary(gen); err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif !reflect.DeepEqual(out.A, in.A) || out.B != in.B || !reflect.DeepEqual(out.C, in.C) {\n\t\tt.Fatalf(\"round trip mismatch: %+v\", out)\n\t}\n}\n"
	genBytelenCase(t, "p", typesSrc, "Arrays", testSrc)
}
