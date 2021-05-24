# binarystruct

Package binarystruct is an automatic type-converting binary data marshaller/unmarshaller for go-language structs.

Go's built-in binary encoding package, "encoding/binary" is the preferred method to deal with binary data structures. The binary package is quite easy to use, but some cases require additional type conversions when binary data are tightly packed.
For example, an integer value in raw binary structure could be stored as a word or a byte, but the decoded value would be type-casted to an architecture-dependent integer value to use in the Go language context.

This package simplifies the typecasting burdens by automatically handling conversion of struct fields using field tags.


## Quick example

Assume we have a binary data structure with a magic header and three integers of byte, word, dword each, like below.
By writing binary data types to field tags in Go struct definition, the values are automatically recognized and converted to proper Go values.

```
// source binary data
blob := []byte { 0x61, 0x62, 0x63, 0x64,
	0x01,
	0x00, 0x02,
	0x00, 0x00, 0x00, 0x03 }
// [ "abcd", 0x01, 0x0002, 0x00000003 ]

// Go struct, with field types specified in tags
strc := struct {
	Header       string `binary:"[4]byte"` // mapped to 4 bytes
	ValueInt8    int    `binary:"int8"`    // mapped to single signed byte
	ValueUint16  int    `binary:"uint16"`  // mapped to two bytes
	ValueDword32 int    `binary:"dword"`   // mapped to four bytes
}{}

// Unmarshal binary data into the struct
readsz, err := binarystruct.Unmarshal(blob, binarystruct.BigEndian, &strc)

// the structure have proper values now
fmt.Println(strc)
// {abcd 1 2 3}

```


## See also
See [go document](https://pkg.go.dev/github.com/mixcode/binarystruct) for details.
