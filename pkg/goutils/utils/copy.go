package utils

import "reflect"

// Creates an exact duplicae of struct behind the given interface
//
// in: source - data structure to create a copy of
// out: a copy of the given source data structure
func Copy(source interface{}) interface{} {

	value := reflect.ValueOf(source)
	switch value.Kind() {
	case reflect.Map:
		return copyMap(source.(map[string]interface{}))
	case reflect.Array, reflect.Slice:
		return copyArray(source.([]interface{}))
	}

	return source
}

func copyMap(m map[string]interface{}) map[string]interface{} {

	copy := make(map[string]interface{})
	for k, v := range m {
		copy[k] = Copy(v)
	}
	return copy
}

func copyArray(a []interface{}) []interface{} {

	copy := make([]interface{}, len(a))
	for i, v := range a {
		copy[i] = Copy(v)
	}
	return copy
}
