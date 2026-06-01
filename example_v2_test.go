// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	"github.com/mixcode/binarystruct"
	"golang.org/x/text/encoding/japanese"
)

// VarintSerializer encodes and decodes integer values as 7-bit varints.
type VarintSerializer struct{}

func (vs *VarintSerializer) Serialize(w io.Writer, value interface{}, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (n int, err error) {
	v, ok := value.(int)
	if !ok {
		return 0, fmt.Errorf("expected int")
	}
	var buf [10]byte
	length := binary.PutUvarint(buf[:], uint64(v))
	return w.Write(buf[:length])
}

func (vs *VarintSerializer) Deserialize(r io.Reader, parentStruct reflect.Value, fieldIndex int, order binarystruct.ByteOrder) (value interface{}, n int, err error) {
	var val uint64
	var shift uint
	var bytesRead int
	if br, ok := r.(io.ByteReader); ok {
		for {
			b, err := br.ReadByte()
			if err != nil {
				return nil, bytesRead, err
			}
			bytesRead++
			val |= uint64(b&0x7f) << shift
			if b&0x80 == 0 {
				break
			}
			shift += 7
		}
	} else {
		var b [1]byte
		for {
			_, err := r.Read(b[:])
			if err != nil {
				return nil, bytesRead, err
			}
			bytesRead++
			val |= uint64(b[0]&0x7f) << shift
			if b[0]&0x80 == 0 {
				break
			}
			shift += 7
		}
	}
	return int(val), bytesRead, nil
}

func Example_v2Features() {
	// Initialize a Marshaller with v2 features (DefaultTextEncoding, Custom Serializers)
	marshaller := &binarystruct.Marshaller{
		DefaultTextEncoding: "sjis",
	}
	marshaller.AddTextEncoding("sjis", japanese.ShiftJIS)
	marshaller.AddSerializer("varint", &VarintSerializer{})

	type Packet struct {
		// Explicit Endian Marking: independent of active byte order
		Length uint32 `binary:"uint32,endian=big"`

		// Custom Serializer: uses the registered "varint" serializer
		Count int `binary:"custom,serializer=varint"`

		// Default Text Encoding Fallback: automatically uses Shift-JIS because no encoding tag is specified
		Name string `binary:"wstring"`
	}

	in := Packet{
		Length: 0x12345678,
		Count:  300, // encoded as [0xac, 0x02] in varint
		Name:   "峠",  // "峠" in Shift-JIS is [0x93, 0xbb]
	}

	// Marshal structural data
	blob, err := marshaller.Marshal(in, binarystruct.LittleEndian)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Blob: %x\n", blob)

	// Unmarshal back
	var out Packet
	_, err = marshaller.Unmarshal(blob, binarystruct.LittleEndian, &out)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Restored: Length=%x, Count=%d, Name=%s\n", out.Length, out.Count, out.Name)

	// One-Value Marshalling/Unmarshalling using MarshalAs and UnmarshalAs
	singleVal := int(9876)
	singleBlob, err := binarystruct.MarshalAs(singleVal, "uint16,endian=big", binarystruct.LittleEndian)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Single Value Blob: %x\n", singleBlob)

	var restoredVal int
	_, err = binarystruct.UnmarshalAs(singleBlob, "uint16,endian=big", binarystruct.LittleEndian, &restoredVal)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Restored Single Value: %d\n", restoredVal)

	// Output:
	// Blob: 12345678ac02020093bb
	// Restored: Length=12345678, Count=300, Name=峠
	// Single Value Blob: 2694
	// Restored Single Value: 9876
}
