

# TODO?


## multidimensional array
	struct {
		MArr [][][]int	`binary:"[4][2][2]int8"`
	}


## custom serializer

	struct {
		VariableSizeInt int	`binary:"[]custom(),serializer=[Serializer_Name]"`
	}

	func (ms *Marshaller) AddSerializer(name string, serializer Serializer)

	Interface Serializer {
		func Serialize(w io.Writer, value interface{}, parentStruct reflect.Value, fieldIndex int, ByteOrder) (sz, err)
		func Deserialize(r io.Reader, parentStruct reflect.Value, fieldIndex int, ByteOrder) (value interface{}, sz, err)
	}


## Write a function to print offset and size of struct fields?


