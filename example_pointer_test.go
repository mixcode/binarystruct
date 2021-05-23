// Copyright 2021 mixcode@github

package binarystruct_test

import (
	"fmt"

	"github.com/mixcode/binarystruct"
)

// struct with pointers
type subStruct struct {
	N int16
	M *int16
}

type exampleStruct struct {
	P1 *int16
	P2 **int16
	P3 *subStruct  // a pointer to another struct
	I  interface{} // an interface
}

// Pointers and interfaces are automatically allocated if possible.
func Example_pointers() {

	// build a struct with pointers
	n1, n2, m1, m2 := int16(1), int16(2), int16(4), int16(6)
	p1, p2, p3, p4 := &n1, &n2, &m1, &m2

	src := exampleStruct{
		P1: p1, // the final value of pointers and interfaces will be written
		P2: &p2,
		P3: &subStruct{3, p3},
		I:  &subStruct{5, p4},
	}

	// marshalling a struct to []byte
	// pointers and interfaces are traversed when marshaled
	data, err := binarystruct.Marshal(&src, binarystruct.LittleEndian)
	if err != nil {
		panic(err)
	}

	// marshaled result:
	// ---
	// +0000  01 00 00 00 02 00 00 00  03 00 00 00 04 00 00 00
	// +0010  05 00 00 00 06 00 00 00
	// ---

	// unmarshalling the data
	r1 := int16(0)
	pr1 := &r1
	restored := exampleStruct{
		P1: nil,          // intermediate variables may be alloced for nil pointers
		P2: &pr1,         // if a value is set to the pointer, then the value will be overwritten
		P3: nil,          // structs are also allocated
		I:  &subStruct{}, // Interfaces must be pre-set to be unmarshaled
	}
	readsz, err := binarystruct.Unmarshal(data, binarystruct.LittleEndian, &restored)
	if err != nil {
		panic(err)
	}

	if readsz != len(data) {
		panic(fmt.Errorf("read and write size does not match: read %d, write %d", readsz, len(data)))
	}

	// print unmarshaled data
	fmt.Printf("%d %d %d %d %d %d\n",
		*restored.P1, **restored.P2,
		restored.P3.N, *restored.P3.M,
		restored.I.(*subStruct).N, *(restored.I.(*subStruct).M),
	)
	fmt.Println(*pr1)

	// Output:
	// 1 2 3 4 5 6
	// 2
}
