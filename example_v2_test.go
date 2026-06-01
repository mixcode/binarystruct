// Copyright 2026 github.com/mixcode

package binarystruct_test

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	"github.com/mixcode/binarystruct"
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

func Example_endianOverride() {
	type Packet struct {
		// Explicit Endian Marking: independent of active byte order
		Length uint32 `binary:"uint32,endian=big"`
		Value  uint32 `binary:"uint32,endian=inverse"`
	}

	in := Packet{
		Length: 0x12345678,
		Value:  0x11223344,
	}

	// Marshal structural data with LittleEndian
	blob, err := binarystruct.Marshal(in, binarystruct.LittleEndian)
	if err != nil {
		panic(err)
	}

	// Length is always BigEndian: 12 34 56 78
	// Value is inverse of LittleEndian -> BigEndian: 11 22 33 44
	fmt.Printf("Blob: %x\n", blob)

	var restored Packet
	_, err = binarystruct.Unmarshal(blob, binarystruct.LittleEndian, &restored)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Restored: Length=%x, Value=%x\n", restored.Length, restored.Value)

	// Output:
	// Blob: 1234567811223344
	// Restored: Length=12345678, Value=11223344
}

func ExampleMarshaller_AddSerializer() {
	marshaller := new(binarystruct.Marshaller)
	marshaller.AddSerializer("varint", &VarintSerializer{})

	type Packet struct {
		// Custom Serializer: uses the registered "varint" serializer
		Count int `binary:"custom,serializer=varint"`
	}

	in := Packet{
		Count: 300,
	}

	// Marshal structural data
	blob, err := marshaller.Marshal(in, binarystruct.LittleEndian)
	if err != nil {
		panic(err)
	}

	// 300 in varint is [0xac, 0x02]
	fmt.Printf("Blob: %x\n", blob)

	var restored Packet
	_, err = marshaller.Unmarshal(blob, binarystruct.LittleEndian, &restored)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Restored: Count=%d\n", restored.Count)

	// Output:
	// Blob: ac02
	// Restored: Count=300
}
