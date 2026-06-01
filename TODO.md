

## TODO ideas

### Benchmarks and advanced optimizers, ideas like precompiled P-code based decoder/encoder, or type and endian converter with SIMD assemblies.

### Explicit endian marking in field tags: "big-endian", "little-endian", "inverse-endian"

### add 'omittable' or 'optional' field tag, mainly for the fields at the end of struct.

### Default text encoding setting

### add mul/div and parenthesis calculation to member size calculator

### one-value marshaller/unmarshaller for non-struct variables

```
// MarshalAs encodes a go value into binary data using suppried tag
func MarshalAs(govalue interface{}, tag string, order ByteOrder) (encoded []byte, err error) {...}

var a []int
UnmarshalAs(a, "[4]byte", bst.LittleEndian)	// read [4]byte to []int
```

### multidimensional array
	struct {
		MArr [][][]int	`binary:"[4][2][2]int8"`
	}


### custom serializer

	struct {
		VariableSizeInt int	`binary:"[]custom(),serializer=[Serializer_Name]"`
	}

	func (ms *Marshaller) AddSerializer(name string, serializer Serializer)

	Interface Serializer {
		func Serialize(w io.Writer, value interface{}, parentStruct reflect.Value, fieldIndex int, ByteOrder) (sz, err)
		func Deserialize(r io.Reader, parentStruct reflect.Value, fieldIndex int, ByteOrder) (value interface{}, sz, err)
	}


### Write a function to print the offset and the size of struct fields?


