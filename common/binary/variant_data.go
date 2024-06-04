package binary

import (
	"bufio"
	"errors"
	"io"
	"reflect"

	E "github.com/sagernet/sing/common/exceptions"
)

func ReadDataSlice(r *bufio.Reader, order ByteOrder, data ...any) error {
	for index, item := range data {
		err := ReadData(r, order, item)
		if err != nil {
			return E.Cause(err, "[", index, "]")
		}
	}
	return nil
}

func ReadData(r *bufio.Reader, order ByteOrder, data any) error {
	switch dataPtr := data.(type) {
	case *[]uint8:
		bytesLen, err := ReadUvarint(r)
		if err != nil {
			return E.Cause(err, "bytes length")
		}
		newBytes := make([]uint8, bytesLen)
		_, err = io.ReadFull(r, newBytes)
		if err != nil {
			return E.Cause(err, "bytes value")
		}
		*dataPtr = newBytes
	default:
		if intBaseDataSize(data) != 0 {
			return Read(r, order, data)
		}
	}
	dataValue := reflect.ValueOf(data)
	if dataValue.Kind() == reflect.Pointer {
		dataValue = dataValue.Elem()
	}
	return readData(r, order, dataValue)
}

func readData(r *bufio.Reader, order ByteOrder, data reflect.Value) error {
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
		for i := 0; i < arrayLen; i++ {
			err := readData(r, order, data.Index(i))
			if err != nil {
				return E.Cause(err, "[", i, "]")
			}
		}
	case reflect.Slice:
		sliceLength, err := ReadUvarint(r)
		if err != nil {
			return E.Cause(err, "slice length")
		}
		if !data.IsNil() && data.Cap() >= int(sliceLength) {
			data.SetLen(int(sliceLength))
		} else if sliceLength > 0 {
			data.Set(reflect.MakeSlice(data.Type(), int(sliceLength), int(sliceLength)))
		}
		if sliceLength > 0 {
			if data.Type().Elem().Kind() == reflect.Uint8 {
				_, err = io.ReadFull(r, data.Bytes())
				if err != nil {
					return E.Cause(err, "bytes value")
				}
			} else {
				for index := 0; index < int(sliceLength); index++ {
					err = readData(r, order, data.Index(index))
					if err != nil {
						return E.Cause(err, "[", index, "]")
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

func WriteDataSlice(writer *bufio.Writer, order ByteOrder, data ...any) error {
	for index, item := range data {
		err := WriteData(writer, order, item)
		if err != nil {
			return E.Cause(err, "[", index, "]")
		}
	}
	return nil
}

func WriteData(writer *bufio.Writer, order ByteOrder, data any) error {
	switch dataPtr := data.(type) {
	case []uint8:
		_, err := writer.Write(AppendUvarint(writer.AvailableBuffer(), uint64(len(dataPtr))))
		if err != nil {
			return E.Cause(err, "bytes length")
		}
		_, err = writer.Write(dataPtr)
		if err != nil {
			return E.Cause(err, "bytes value")
		}
	default:
		if intBaseDataSize(data) != 0 {
			return Write(writer, order, data)
		}
	}
	return writeData(writer, order, reflect.Indirect(reflect.ValueOf(data)))
}

func writeData(writer *bufio.Writer, order ByteOrder, data reflect.Value) error {
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
		_, err := writer.Write(AppendUvarint(writer.AvailableBuffer(), uint64(len(stringValue))))
		if err != nil {
			return E.Cause(err, "string length")
		}
		if stringValue != "" {
			_, err = writer.WriteString(stringValue)
			if err != nil {
				return E.Cause(err, "string value")
			}
		}
	case reflect.Array:
		dataLen := data.Len()
		for i := 0; i < dataLen; i++ {
			err := writeData(writer, order, data.Index(i))
			if err != nil {
				return E.Cause(err, "[", i, "]")
			}
		}
	case reflect.Slice:
		dataLen := data.Len()
		_, err := writer.Write(AppendUvarint(writer.AvailableBuffer(), uint64(dataLen)))
		if err != nil {
			return E.Cause(err, "slice length")
		}
		if dataLen > 0 {
			if data.Type().Elem().Kind() == reflect.Uint8 {
				_, err = writer.Write(data.Bytes())
				if err != nil {
					return E.Cause(err, "bytes value")
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
		_, err := writer.Write(AppendUvarint(writer.AvailableBuffer(), uint64(dataLen)))
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

func intBaseDataSize(data any) int {
	switch data.(type) {
	case bool, int8, uint8:
		return 1
	case int16, uint16:
		return 2
	case int32, uint32:
		return 4
	case int64, uint64:
		return 8
	case float32:
		return 4
	case float64:
		return 8
	}
	return 0
}
