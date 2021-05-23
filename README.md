# binarystruct

Package binarystruct is an automatic type-converting binary data marshaller/unmarshaller for go-language structs.

Go's built-in binary encoding package, "encoding/binary" is the preferred method to deal with binary data structures. The binary package is quite easy to use, but some cases require additional type conversions when binary data are tightly packed.
For example, an integer value in raw binary structure could be stored as a word or a byte, but the decoded value would be type-casted to an architecture-dependent integer value to use in the Go language context.

This package simplifies the typecasting burdens by automatically handling conversion of struct fields by reading field tags.


## Quick example

For example, a binary data struct may have a magic header and three integers of byte, word, dword each. By writing field tags in Go struct definition, the binary values are automatically recognized and converted to proper Go values.

```
// source binary data
blob := []byte { 0x61, 0x62, 0x63, 0x64,
	0x01,
	0x00, 0x02,
	0x00, 0x00, 0x00, 0x03 }
// [ "ABCD", 0x01, 0x0002, 0x00000003 ]

// Go struct, with field types specified in tags
strc := struct {
	Header       string `binary:"[4]byte"` // marshaled to 4 bytes
	ValueInt8    int    `binary:"int8"`    // marshaled to single byte
	ValueUint16  int    `binary:"uint16"`  // marshaled to two bytes
	ValueDword32 int    `binary:"dword"`   // marshaled to four bytes
}

// Unmarshal binary data into the struct
readsz, err := binarystruct.Unmarshal(blob, binarystruct.BigEndian, &strc)

// the structure have proper values now
fmt.Println(strc)
// {abcd 1 2 3}

```


## See alse
See [go document]() for details.
