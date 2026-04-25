package json

import "reflect"

func ObjectKeys(object reflect.Type) []string {
	switch object.Kind() {
	case reflect.Pointer:
		return ObjectKeys(object.Elem())
	case reflect.Struct:
	default:
		panic("invalid non-struct input")
	}
	fields := cachedTypeFields(object).list
	keys := make([]string, len(fields))
	for i, field := range fields {
		keys[i] = field.name
	}
	return keys
}
