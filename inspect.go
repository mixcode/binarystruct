// Copyright 2026 github.com/mixcode

package binarystruct

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// FieldLayout holds layout details of a serialized struct field.
type FieldLayout struct {
	Index      int         `json:"index"`
	Name       string      `json:"name"`
	GoType     string      `json:"go_type"`
	BinaryType string      `json:"binary_type"`
	Offset     int         `json:"offset"`              // Offset from the start of the struct (in bytes)
	Size       int         `json:"size"`                // Encoded size of the field (in bytes)
	Tag        string      `json:"tag"`                 // Raw binary tag
	Endian     string      `json:"endian"`              // Byte order representation
	RawValue   interface{} `json:"raw_value,omitempty"` // Field's current value
	Details    string      `json:"details,omitempty"`   // Dynamic expressions, omission reason, etc.
}

// LayoutFormat holds format configurations for ASCII table generation.
type LayoutFormat struct {
	OffsetBase int // Base to format offsets (e.g. 10 for decimal, 16 for hex)
	SizeBase   int // Base to format sizes (e.g. 10 for decimal, 16 for hex)
	ValueBase  int // Base to format scalar numeric values (e.g. 10 or 16)
}

// DefaultLayoutFormat is the default layout configuration.
var DefaultLayoutFormat = LayoutFormat{
	OffsetBase: 10,
	SizeBase:   10,
	ValueBase:  10,
}

// StructLayout holds layout details of a serialized struct.
type StructLayout struct {
	TypeName  string        `json:"type_name"`
	TotalSize int           `json:"total_size"`
	Fields    []FieldLayout `json:"fields"`
}

// String returns a formatted ASCII table using the DefaultLayoutFormat.
func (sl *StructLayout) String() string {
	return sl.Format(DefaultLayoutFormat)
}

// ToJSON returns the JSON encoding of the struct layout.
func (sl *StructLayout) ToJSON() ([]byte, error) {
	return json.MarshalIndent(sl, "", "\t")
}

// Format returns a formatted ASCII table using custom formatting options.
func (sl *StructLayout) Format(cfg LayoutFormat) string {
	type row struct {
		offset  string
		size    string
		name    string
		goType  string
		binType string
		endian  string
		value   string
		details string
	}

	rows := make([]row, len(sl.Fields))
	maxOffset := len("OFFSET")
	maxSize := len("SIZE")
	maxName := len("FIELD NAME")
	maxGoType := len("GO TYPE")
	maxBinType := len("BINARY TYPE")
	maxEndian := len("ENDIAN")
	maxValue := len("VALUE")

	for i, f := range sl.Fields {
		r := row{
			offset:  formatInt(f.Offset, cfg.OffsetBase),
			size:    formatInt(f.Size, cfg.SizeBase),
			name:    f.Name,
			goType:  f.GoType,
			binType: f.BinaryType,
			endian:  f.Endian,
			value:   formatValue(f.RawValue, cfg.ValueBase),
			details: f.Details,
		}
		if len(r.offset) > maxOffset {
			maxOffset = len(r.offset)
		}
		if len(r.size) > maxSize {
			maxSize = len(r.size)
		}
		if len(r.name) > maxName {
			maxName = len(r.name)
		}
		if len(r.goType) > maxGoType {
			maxGoType = len(r.goType)
		}
		if len(r.binType) > maxBinType {
			maxBinType = len(r.binType)
		}
		if len(r.endian) > maxEndian {
			maxEndian = len(r.endian)
		}
		if len(r.value) > maxValue {
			maxValue = len(r.value)
		}
		rows[i] = r
	}

	var sb strings.Builder
	totalSizeStr := formatInt(sl.TotalSize, cfg.SizeBase)
	sb.WriteString(fmt.Sprintf("Struct Layout: %s (Total Size: %s)\n", sl.TypeName, totalSizeStr))

	headerPattern := fmt.Sprintf("%%-%ds   %%-%ds   %%-%ds   %%-%ds   %%-%ds   %%-%ds   %%-%ds   %%s\n",
		maxOffset, maxSize, maxName, maxGoType, maxBinType, maxEndian, maxValue)

	headerText := fmt.Sprintf(headerPattern, "OFFSET", "SIZE", "FIELD NAME", "GO TYPE", "BINARY TYPE", "ENDIAN", "VALUE", "DETAILS")
	dividerLen := len(headerText) + 12
	if dividerLen < 80 {
		dividerLen = 80
	}
	divider := strings.Repeat("=", dividerLen)
	separator := strings.Repeat("-", dividerLen)

	sb.WriteString(divider + "\n")
	sb.WriteString(strings.TrimRight(headerText, " \t\r\n") + "\n")
	sb.WriteString(separator + "\n")
	for _, r := range rows {
		rowStr := fmt.Sprintf(headerPattern, r.offset, r.size, r.name, r.goType, r.binType, r.endian, r.value, r.details)
		sb.WriteString(strings.TrimRight(rowStr, " \t\r\n") + "\n")
	}
	sb.WriteString(divider + "\n")

	return sb.String()
}

// Inspect analyzes a struct instance and returns its exact binary layout.
func Inspect(govalue interface{}, order ByteOrder) (*StructLayout, error) {
	return NewMarshaler(order).Inspect(govalue)
}

// Inspect analyzes a struct instance using this Marshaler's byte order and custom configuration (e.g. codecs).
func (ms *Marshaler) Inspect(govalue interface{}) (*StructLayout, error) {
	order, err := ms.effectiveOrder()
	if err != nil {
		return nil, err
	}
	v := reflect.ValueOf(govalue)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, fmt.Errorf("cannot inspect nil value")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("unsupported type: %v (must be a struct)", v.Kind())
	}

	sl := &StructLayout{
		TypeName: v.Type().Name(),
	}

	offset := 0
	err = ms.inspectStruct(v, order, "", &sl.Fields, &offset)
	if err != nil {
		return nil, err
	}
	sl.TotalSize = offset

	return sl, nil
}

func (ms *Marshaler) inspectStruct(strc reflect.Value, order ByteOrder, prefix string, fields *[]FieldLayout, offset *int) error {
	typ := strc.Type()
	meta, err := getStructMetadata(typ)
	if err != nil {
		return err
	}

	omittedRemaining := false
	for _, fMeta := range meta.fields {
		if fMeta.ignore {
			continue
		}
		if fMeta.unexported {
			continue
		}

		fieldName := fMeta.name
		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		tagStr := typ.Field(fMeta.index).Tag.Get(tagName)

		if omittedRemaining {
			*fields = append(*fields, FieldLayout{
				Index:      fMeta.index,
				Name:       fieldName,
				GoType:     typ.Field(fMeta.index).Type.String(),
				BinaryType: fMeta.encodeType.String(),
				Offset:     *offset,
				Size:       0,
				Tag:        tagStr,
				Endian:     resolveByteOrder(order, fMeta.endian).String(),
				RawValue:   nil,
				Details:    "omitted (subsequent to an omitted field)",
			})
			continue
		}

		// Check if omittable expression is met
		if fMeta.omittable && fMeta.omittableExpr != "" {
			limit, errEval := evaluateTagValue(strc, fMeta.omittableExpr)
			if errEval == nil && *offset >= limit {
				*fields = append(*fields, FieldLayout{
					Index:      fMeta.index,
					Name:       fieldName,
					GoType:     typ.Field(fMeta.index).Type.String(),
					BinaryType: fMeta.encodeType.String(),
					Offset:     *offset,
					Size:       0,
					Tag:        tagStr,
					Endian:     resolveByteOrder(order, fMeta.endian).String(),
					RawValue:   nil,
					Details:    fmt.Sprintf("omitted (reached limit %d)", limit),
				})
				omittedRemaining = true
				continue
			}
		}

		fieldVal := strc.Field(fMeta.index)
		fKind := typ.Field(fMeta.index).Type.Kind()

		// Check if omittable nil pointer
		if fMeta.omittable && (fKind == reflect.Ptr || fKind == reflect.Interface) && fieldVal.IsNil() {
			*fields = append(*fields, FieldLayout{
				Index:      fMeta.index,
				Name:       fieldName,
				GoType:     typ.Field(fMeta.index).Type.String(),
				BinaryType: fMeta.encodeType.String(),
				Offset:     *offset,
				Size:       0,
				Tag:        tagStr,
				Endian:     resolveByteOrder(order, fMeta.endian).String(),
				RawValue:   nil,
				Details:    "omitted (pointer is nil)",
			})
			omittedRemaining = true
			continue
		}

		// Determine natural type and options
		var naturalType eType
		var option typeOption
		if fieldVal.IsValid() {
			naturalType, option = getNaturalType(fieldVal)
		}
		if fMeta.hasTag {
			if fMeta.encodeType != Any {
				naturalType = fMeta.encodeType
			}
			if fMeta.isArray {
				option.isArray = true
				if fMeta.arrayLenExpr != "" {
					option.arrayLen, _ = evaluateTagValue(strc, fMeta.arrayLenExpr)
				}
			}
			if fMeta.bufLenExpr != "" {
				option.bufLen, _ = evaluateTagValue(strc, fMeta.bufLenExpr)
			}
			if fMeta.encoding != "" {
				option.encoding = fMeta.encoding
			}
			if fMeta.endian != endianNone {
				option.endian = fMeta.endian
			}
			if fMeta.codec != "" {
				option.codec = fMeta.codec
			}
		}

		fieldOrder := resolveByteOrder(order, option.endian)

		// Dereference pointer/interface for size/nested analysis
		v := fieldVal
		for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
			if v.IsNil() {
				break
			}
			v = v.Elem()
		}

		// Calculate size of this field
		size := 0
		details := ""

		if option.codec != "" {
			if ms.codecs != nil {
				codec, ok := ms.codecs[option.codec]
				if ok && v.IsValid() {
					var buf bytes.Buffer
					nWritten, errSer := codec.Encode(&buf, fieldVal.Interface(), strc, fMeta.index, fieldOrder)
					if errSer == nil {
						size = nWritten
					}
				}
			}
			details = "custom codec: " + option.codec
		} else if v.IsValid() && v.Kind() == reflect.Struct && naturalType == iStruct {
			// Nested struct recursion
			err := ms.inspectStruct(v, fieldOrder, fieldName, fields, offset)
			if err != nil {
				return err
			}
			continue
		} else {
			size = calculateFieldSize(v, naturalType, option)
			if fMeta.hasTag {
				if fMeta.arrayLenExpr != "" {
					details = fmt.Sprintf("expr: %s", fMeta.arrayLenExpr)
				} else if fMeta.bufLenExpr != "" {
					details = fmt.Sprintf("expr: %s", fMeta.bufLenExpr)
				}
			}
		}

		*fields = append(*fields, FieldLayout{
			Index:      fMeta.index,
			Name:       fieldName,
			GoType:     typ.Field(fMeta.index).Type.String(),
			BinaryType: naturalType.String(),
			Offset:     *offset,
			Size:       size,
			Tag:        tagStr,
			Endian:     fieldOrder.String(),
			RawValue:   fieldVal.Interface(),
			Details:    details,
		})

		*offset += size
	}
	return nil
}

func calculateFieldSize(v reflect.Value, k eType, option typeOption) int {
	if option.isArray {
		elementSize := k.ByteSize()
		if elementSize > 0 {
			return option.arrayLen * elementSize
		}
		if v.IsValid() && (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) {
			sz := 0
			for i := 0; i < v.Len(); i++ {
				sz += calculateFieldSize(v.Index(i), k, typeOption{})
			}
			return sz
		}
		return 0
	}

	if k == Pad {
		return option.bufLen
	}

	if k == String || k == Bstring || k == Wstring || k == Dwstring || k == Zstring || k == Z16string {
		if option.bufLen > 0 {
			return option.bufLen
		}
		if v.IsValid() && v.Kind() == reflect.String {
			strLen := len(v.String())
			switch k {
			case Bstring:
				return strLen + 1
			case Wstring:
				return strLen + 2
			case Dwstring:
				return strLen + 4
			case Zstring:
				return strLen + 1
			case Z16string:
				return strLen + 2
			default:
				return strLen
			}
		}
		return 0
	}

	return k.ByteSize()
}

func formatInt(val int, base int) string {
	if base == 16 {
		return fmt.Sprintf("0x%x", val)
	}
	return fmt.Sprintf("%d", val)
}

func formatValue(val interface{}, base int) string {
	if val == nil {
		return "<nil>"
	}
	v := reflect.ValueOf(val)
	if !v.IsValid() {
		return "<nil>"
	}

	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "<nil>"
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if base == 16 {
			return fmt.Sprintf("0x%x", v.Int())
		}
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if base == 16 {
			return fmt.Sprintf("0x%x", v.Uint())
		}
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			var b []byte
			if v.Kind() == reflect.Slice {
				b = v.Bytes()
			} else {
				b = make([]byte, v.Len())
				for i := 0; i < v.Len(); i++ {
					b[i] = byte(v.Index(i).Uint())
				}
			}
			if len(b) > 4 {
				if base == 16 {
					return fmt.Sprintf("[0x%x 0x%x 0x%x...]", b[0], b[1], b[2])
				}
				return fmt.Sprintf("[%d %d %d...]", b[0], b[1], b[2])
			}
			var parts []string
			for _, x := range b {
				if base == 16 {
					parts = append(parts, fmt.Sprintf("0x%x", x))
				} else {
					parts = append(parts, fmt.Sprintf("%d", x))
				}
			}
			return "[" + strings.Join(parts, " ") + "]"
		}

		var parts []string
		length := v.Len()
		limit := length
		if limit > 3 {
			limit = 3
		}
		for i := 0; i < limit; i++ {
			parts = append(parts, formatValue(v.Index(i).Interface(), base))
		}
		if length > 3 {
			parts = append(parts, "...")
		}
		return "[" + strings.Join(parts, " ") + "]"

	case reflect.String:
		s := v.String()
		if len(s) > 12 {
			return fmt.Sprintf("%q...", s[:12])
		}
		return fmt.Sprintf("%q", s)
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}
