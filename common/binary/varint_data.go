package binary

import (
	"errors"
	"io"
	"reflect"

	E "github.com/sagernet/sing/common/exceptions"
)

type Reader interface {
	io.Reader
	io.ByteReader
}

type Writer interface {
	io.Writer
	io.ByteWriter
}

func ReadData(r Reader, order ByteOrder, rawData any) error {
	switch data := rawData.(type) {
	case *[]bool:
		return readBaseData(r, order, data)
	case *[]int8:
		return readBaseData(r, order, data)
	case *[]uint8:
		return readBaseData(r, order, data)
	case *[]int16:
		return readBaseData(r, order, data)
	case *[]uint16:
		return readBaseData(r, order, data)
	case *[]int32:
		return readBaseData(r, order, data)
	case *[]uint32:
		return readBaseData(r, order, data)
	case *[]int64:
		return readBaseData(r, order, data)
	case *[]uint64:
		return readBaseData(r, order, data)
	case *[]float32:
		return readBaseData(r, order, data)
	case *[]float64:
		return readBaseData(r, order, data)
	default:
		if intBaseDataSize(rawData) != 0 {
			return Read(r, order, rawData)
		}
	}
	return readData(r, order, reflect.Indirect(reflect.ValueOf(rawData)))
}

func readBaseData[T any](r Reader, order ByteOrder, data *[]T) error {
	dataLen, err := ReadUvarint(r)
	if err != nil {
		return E.Cause(err, "slice length")
	}
	if dataLen == 0 {
		*data = nil
		return nil
	}
	dataSlices := make([]T, dataLen)
	err = Read(r, order, dataSlices)
	if err != nil {
		return err
	}
	*data = dataSlices
	return nil
}

func readData(r Reader, order ByteOrder, data reflect.Value) error {
	switch data.Kind() {
	case reflect.Pointer:
		pointerValue, err := r.ReadByte()
		if err != nil {
			return err
		}
		if pointerValue == 0 {
			data.SetZero()
			return nil
		}
		if data.IsNil() {
			data.Set(reflect.New(data.Type().Elem()))
		}
		return readData(r, order, data.Elem())
	case reflect.String:
		stringLength, err := ReadUvarint(r)
		if err != nil {
			return E.Cause(err, "string length")
		}
		if stringLength == 0 {
			data.SetZero()
		} else {
			stringData := make([]byte, stringLength)
			_, err = io.ReadFull(r, stringData)
			if err != nil {
				return E.Cause(err, "string value")
			}
			data.SetString(string(stringData))
		}
	case reflect.Array:
		arrayLen := data.Len()
		itemSize := sizeof(data.Type())
		if itemSize > 0 {
			buf := make([]byte, itemSize*arrayLen)
			_, err := io.ReadFull(r, buf)
			if err != nil {
				return err
			}
			d := &decoder{order: order, buf: buf}
			d.value(data)
		} else {
			for i := 0; i < arrayLen; i++ {
				err := readData(r, order, data.Index(i))
				if err != nil {
					return E.Cause(err, "[", i, "]")
				}
			}
		}
	case reflect.Slice:
		sliceLength, err := ReadUvarint(r)
		if err != nil {
			return E.Cause(err, "slice length")
		}
		if sliceLength == 0 {
			data.SetZero()
		} else {
			dataSlices := makeBaseDataSlices(data, int(sliceLength))
			if dataSlices != nil {
				err = Read(r, order, dataSlices)
				if err != nil {
					return err
				}
				setBaseDataSlices(data, dataSlices)
			} else {
				if !data.IsNil() && data.Cap() >= int(sliceLength) {
					data.SetLen(int(sliceLength))
				} else if sliceLength > 0 {
					data.Set(reflect.MakeSlice(data.Type(), int(sliceLength), int(sliceLength)))
				}
				for i := 0; i < int(sliceLength); i++ {
					err = readData(r, order, data.Index(i))
					if err != nil {
						return E.Cause(err, "[", i, "]")
					}
				}
			}
		}
	case reflect.Map:
		mapLength, err := ReadUvarint(r)
		if err != nil {
			return E.Cause(err, "map length")
		}
		data.Set(reflect.MakeMap(data.Type()))
		for index := 0; index < int(mapLength); index++ {
			key := reflect.New(data.Type().Key()).Elem()
			err = readData(r, order, key)
			if err != nil {
				return E.Cause(err, "[", index, "].key")
			}
			value := reflect.New(data.Type().Elem()).Elem()
			err = readData(r, order, value)
			if err != nil {
				return E.Cause(err, "[", index, "].value")
			}
			data.SetMapIndex(key, value)
		}
	case reflect.Struct:
		fieldType := data.Type()
		fieldLen := data.NumField()
		for i := 0; i < fieldLen; i++ {
			field := data.Field(i)
			fieldName := fieldType.Field(i).Name
			if field.CanSet() || fieldName != "_" {
				err := readData(r, order, field)
				if err != nil {
					return E.Cause(err, fieldName)
				}
			}
		}
	default:
		size := dataSize(data)
		if size < 0 {
			return errors.New("invalid type " + reflect.TypeOf(data).String())
		}
		d := &decoder{order: order, buf: make([]byte, size)}
		_, err := io.ReadFull(r, d.buf)
		if err != nil {
			return err
		}
		d.value(data)
	}
	return nil
}

func WriteData(writer Writer, order ByteOrder, rawData any) error {
	switch data := rawData.(type) {
	case []bool:
		return writeBaseData(writer, order, data)
	case []int8:
		return writeBaseData(writer, order, data)
	case []uint8:
		return writeBaseData(writer, order, data)
	case []int16:
		return writeBaseData(writer, order, data)
	case []uint16:
		return writeBaseData(writer, order, data)
	case []int32:
		return writeBaseData(writer, order, data)
	case []uint32:
		return writeBaseData(writer, order, data)
	case []int64:
		return writeBaseData(writer, order, data)
	case []uint64:
		return writeBaseData(writer, order, data)
	case []float32:
		return writeBaseData(writer, order, data)
	case []float64:
		return writeBaseData(writer, order, data)
	default:
		if intBaseDataSize(rawData) != 0 {
			return Write(writer, order, rawData)
		}
	}
	return writeData(writer, order, reflect.Indirect(reflect.ValueOf(rawData)))
}

func writeBaseData[T any](writer Writer, order ByteOrder, data []T) error {
	_, err := WriteUvarint(writer, uint64(len(data)))
	if err != nil {
		return err
	}
	if len(data) > 0 {
		return Write(writer, order, data)
	}
	return nil
}

func writeData(writer Writer, order ByteOrder, data reflect.Value) error {
	switch data.Kind() {
	case reflect.Pointer:
		if data.IsNil() {
			err := writer.WriteByte(0)
			if err != nil {
				return err
			}
		} else {
			err := writer.WriteByte(1)
			if err != nil {
				return err
			}
			return writeData(writer, order, data.Elem())
		}
	case reflect.String:
		stringValue := data.String()
		_, err := WriteUvarint(writer, uint64(len(stringValue)))
		if err != nil {
			return E.Cause(err, "string length")
		}
		if stringValue != "" {
			_, err = writer.Write([]byte(stringValue))
			if err != nil {
				return E.Cause(err, "string value")
			}
		}
	case reflect.Array:
		dataLen := data.Len()
		if dataLen > 0 {
			itemSize := intItemBaseDataSize(data)
			if itemSize > 0 {
				buf := make([]byte, itemSize*dataLen)
				e := &encoder{order: order, buf: buf}
				e.value(data)
				_, err := writer.Write(buf)
				if err != nil {
					return E.Cause(err, reflect.TypeOf(data).String())
				}
			} else {
				for i := 0; i < dataLen; i++ {
					err := writeData(writer, order, data.Index(i))
					if err != nil {
						return E.Cause(err, "[", i, "]")
					}
				}
			}
		}
	case reflect.Slice:
		dataLen := data.Len()
		_, err := WriteUvarint(writer, uint64(dataLen))
		if err != nil {
			return E.Cause(err, "slice length")
		}
		if dataLen > 0 {
			dataSlices := baseDataSlices(data)
			if dataSlices != nil {
				err = Write(writer, order, dataSlices)
				if err != nil {
					return err
				}
			} else {
				for i := 0; i < dataLen; i++ {
					err = writeData(writer, order, data.Index(i))
					if err != nil {
						return E.Cause(err, "[", i, "]")
					}
				}
			}
		}
	case reflect.Map:
		dataLen := data.Len()
		_, err := WriteUvarint(writer, uint64(dataLen))
		if err != nil {
			return E.Cause(err, "map length")
		}
		if dataLen > 0 {
			for index, key := range data.MapKeys() {
				err = writeData(writer, order, key)
				if err != nil {
					return E.Cause(err, "[", index, "].key")
				}
				err = writeData(writer, order, data.MapIndex(key))
				if err != nil {
					return E.Cause(err, "[", index, "].value")
				}
			}
		}
	case reflect.Struct:
		fieldType := data.Type()
		fieldLen := data.NumField()
		for i := 0; i < fieldLen; i++ {
			field := data.Field(i)
			fieldName := fieldType.Field(i).Name
			if field.CanSet() || fieldName != "_" {
				err := writeData(writer, order, field)
				if err != nil {
					return E.Cause(err, fieldName)
				}
			}
		}
	default:
		size := dataSize(data)
		if size < 0 {
			return errors.New("binary.Write: some values are not fixed-sized in type " + data.Type().String())
		}
		buf := make([]byte, size)
		e := &encoder{order: order, buf: buf}
		e.value(data)
		_, err := writer.Write(buf)
		if err != nil {
			return E.Cause(err, reflect.TypeOf(data).String())
		}
	}
	return nil
}

func intItemBaseDataSize(data reflect.Value) int {
	itemType := data.Type().Elem()
	switch itemType.Kind() {
	case reflect.Bool,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return itemType.Len()
	default:
		return -1
	}
}

func intBaseDataSize(data any) int {
	switch data.(type) {
	case bool, int8, uint8,
		*bool, *int8, *uint8:
		return 1
	case int16, uint16, *int16, *uint16:
		return 2
	case int32, uint32, *int32, *uint32:
		return 4
	case int64, uint64, *int64, *uint64:
		return 8
	case float32, *float32:
		return 4
	case float64, *float64:
		return 8
	}
	return 0
}
