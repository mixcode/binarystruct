// Copyright 2026 github.com/mixcode

package binarystruct_test

import "testing"

// TestCodegen_NestedDirectCall_Parity verifies the direct generated-method call
// path for nested structs and slices-of-structs (when the nested type is itself
// generated). The generated MarshalBinary output must be byte-identical to the
// runtime interpreter, and the value must round-trip.
func TestCodegen_NestedDirectCall_Parity(t *testing.T) {
	typesSrc := "type Inner struct {\n" +
		"\tX uint16 `binary:\"uint16\"`\n" +
		"\tY uint8  `binary:\"uint8\"`\n}\n\n" +
		"type Outer struct {\n" +
		"\t_    struct{} `binary:\"endian=big\"`\n" +
		"\tH    Inner    `binary:\"any\"`\n" +
		"\tCnt  uint16   `binary:\"uint16,valueof=count(List)\"`\n" +
		"\tList []Inner  `binary:\"[Cnt]any\"`\n}\n"
	testSrc := "import (\n\t\"bytes\"\n\t\"reflect\"\n\t\"testing\"\n\n\t\"github.com/mixcode/binarystruct\"\n)\n\n" +
		"func TestNested(t *testing.T) {\n" +
		"\tin := Outer{H: Inner{X: 0x0102, Y: 3}, List: []Inner{{X: 10, Y: 11}, {X: 20, Y: 21}}}\n" +
		"\tgen, err := in.MarshalBinary()\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\trt, err := binarystruct.Marshal(&in) // runtime interpreter\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif !bytes.Equal(gen, rt) {\n\t\tt.Fatalf(\"codegen %x vs runtime %x\", gen, rt)\n\t}\n" +
		"\t// H(3) + Cnt(2) + 2*Inner(3) = 11 bytes; H.X big-endian = 01 02.\n" +
		"\tif len(gen) != 11 || gen[0] != 0x01 || gen[1] != 0x02 {\n\t\tt.Fatalf(\"unexpected layout: % x\", gen)\n\t}\n" +
		"\tvar out Outer\n" +
		"\tif err := out.UnmarshalBinary(gen); err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
		"\tif out.H != in.H || !reflect.DeepEqual(out.List, in.List) {\n\t\tt.Fatalf(\"round trip mismatch: %+v\", out)\n\t}\n}\n"
	genBytelenCase(t, "p", typesSrc, "Outer,Inner", testSrc)
}
