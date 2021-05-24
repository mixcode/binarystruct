# binarystruct : binary data encoder/decoder for native Go structs

Package binarystruct is an automatic type-converting binary data encoder/decoder(or marshaller/unmarshaller) for go-language structs.

Go's built-in binary encoding package, "encoding/binary" is the preferred method to deal with binary data structures. The binary package is quite easy to use, but some cases require additional type conversions when values are tightly packed.
For example, an integer value in raw binary structure may be stored as a word or a byte, but the decoded value would be type-casted to an architecture-dependent integer for easy of use in the Go context.

This package simplifies the typecasting burdens by automatically handling conversion of struct fields using field tags.


## A Quick Example

Assume we have a binary data structure with a magic header and three integers, byte, word, dword each, like below.
By writing binary data types to field tags in Go struct definition, the values are automatically recognized and converted to proper encoding types.

```
// source binary data
blob := []byte { 0x61, 0x62, 0x63, 0x64,
	0x01,
	0x00, 0x02,
	0x00, 0x00, 0x00, 0x03 }
// [ "abcd", 0x01, 0x0002, 0x00000003 ]

// Go struct, with field types specified in tags
strc := struct {
	Header       string `binary:"[4]byte"` // maps to 4 bytes
	ValueInt8    int    `binary:"int8"`    // maps to single signed byte
	ValueUint16  int    `binary:"uint16"`  // maps to two bytes
	ValueDword32 int    `binary:"dword"`   // maps to four bytes
}{}

// Unmarshal binary data into the struct
readsz, err := binarystruct.Unmarshal(blob, binarystruct.BigEndian, &strc)

// the structure have proper values now
fmt.Println(strc)
// {abcd 1 2 3}

// Marshal a struct to []byte
output, err := binarystruct.Marshal(&strc, binarystruct.BigEndian)
// output == blob

```


## See also
See [go document](https://pkg.go.dev/github.com/mixcode/binarystruct) for details.
