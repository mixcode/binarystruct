// Copyright 2026 github.com/mixcode

package binarystruct_test

import "testing"

// bulkScalarArraysSrc / bulkScalarTestSrc describe a struct exercising the
// fixed-width multibyte scalar array/slice paths (uint32 slice with a field-ref
// length, a fixed [3]int16 array, and a float32 slice) plus a test asserting the
// generated MarshalBinary output is byte-identical to the runtime interpreter
// (layout + endianness preserved) and that the round trip recovers the value.
const bulkScalarArraysSrc = "type Arrays struct {\n" +
	"\t_ struct{}  `binary:\"endian=big\"`\n" +
	"\tN uint16    `binary:\"uint16,valueof=count(A)\"`\n" +
	"\tA []uint32  `binary:\"[N]uint32\"`\n" +
	"\tB [3]int16  `binary:\"[3]int16\"`\n" +
	"\tM uint16    `binary:\"uint16,valueof=count(C)\"`\n" +
	"\tC []float32 `binary:\"[M]float32\"`\n}\n"

const bulkScalarTestSrc = "import (\n\t\"bytes\"\n\t\"reflect\"\n\t\"testing\"\n\n\t\"github.com/mixcode/binarystruct\"\n)\n\n" +
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

// TestCodegen_BulkScalarSlice_Parity covers the default (portable) per-element
// bulk path.
func TestCodegen_BulkScalarSlice_Parity(t *testing.T) {
	genBytelenCase(t, "p", bulkScalarArraysSrc, "Arrays", bulkScalarTestSrc)
}

// TestCodegen_BulkScalarSlice_UnsafeParity runs the same struct through the
// opt-in -unsafe-bulk raw-memory path (unsafe + in-place SwapBytes). It must
// stay byte-identical to the runtime interpreter and round-trip cleanly — the
// flag only trades portability for speed, never changes the wire format.
func TestCodegen_BulkScalarSlice_UnsafeParity(t *testing.T) {
	genBytelenCase(t, "p", bulkScalarArraysSrc, "Arrays", bulkScalarTestSrc, "-unsafe-bulk")
}
