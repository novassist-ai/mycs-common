package utils

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

// Returns the map from an array of maps with the key matching the given regex.
//
// in: keyPath - key path where path separator is '/'
// in: valueMapArray - nested map of maps to lookup key in
func GetItemsWithMatchAtPath(keyPath, keyValueMatch string, valueMapArray []interface{}) ([]interface{}, error) {

	var (
		err error
		ok  bool

		searchExpression *regexp.Regexp
		keyValue         interface{}
		value            string

		result = []interface{}{}
	)

	if searchExpression, err = regexp.Compile(keyValueMatch); err != nil {
		return nil, err
	}
	for i, m := range valueMapArray {

		if keyValue, err = GetValueAtPath(keyPath, m); err != nil {
			return nil, err
		}
		if value, ok = keyValue.(string); !ok {
			return nil, fmt.Errorf(
				"key at path '%s' of map item at index '%d' is not a string",
				keyPath, i)
		}
		if searchExpression.Match([]byte(value)) {
			result = append(result, m)
		}
	}
	return result, nil
}

// Sets a value at the given path within a nested map instance
//
// in: keyPath - key path where path separator is '/'
// in: value - value to set
// in: valueMap - nested map of maps to lookup key in
func SetValueAtPath(keyPath string, value interface{}, valueMap interface{}) error {

	var (
		err error
		ok  bool

		m map[string]interface{}
		r interface{}
	)

	pathElems := strings.Split(keyPath, "/")
	numPathElems := len(pathElems)

	if numPathElems > 1 {
		if r, err = getValueRefAtPath(pathElems[0:numPathElems-1], valueMap); err != nil {
			return err
		}
		if m, ok = reflect.Indirect(reflect.ValueOf(r)).Interface().(map[string]interface{}); !ok {
			return fmt.Errorf("key's parent is not of type map[string]interface{} type")
		}
		m[pathElems[numPathElems-1]] = value

	} else {
		if m, ok = valueMap.(map[string]interface{}); !ok {
			return fmt.Errorf("given value map object is not of type map[string]interface{}")
		}
		m[pathElems[0]] = value
	}

	return nil
}

func getValueRefAtPath(keyPath []string, valueMap interface{}) (interface{}, error) {

	var (
		ok bool
		m  map[string]interface{}
		v  interface{}
	)

	if m, ok = valueMap.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("given value map object is not of type map[string]interface{}")
	}
	if v, ok = m[keyPath[0]]; !ok {
		// no value at given path so return nil
		return nil, nil
	}
	if len(keyPath) == 1 {
		return &v, nil
	} else {
		// recursively lookup the value
		return getValueRefAtPath(keyPath[1:], v)
	}
}

// Returns the value at the given path within a nested map instance
//
// in: keyPath - key path where path separator is '/'
// in: valueMap - nested map of maps to lookup key in
// out: the value at the given path
func MustGetValueAtPath(keyPath string, valueMap interface{}) interface{} {

	var (
		err   error
		value interface{}
	)

	if value, err = GetValueAtPath(keyPath, valueMap); err != nil {
		logger.TraceMessage("utils.MustGetValueAtPath: %s", err.Error())		
	}
	return value
}

func GetValueAtPath(keyPath string, valueMap interface{}) (interface{}, error) {
	if strings.HasPrefix(keyPath, "/") {
		return getValueAtPath(strings.Split(keyPath[1:], "/"), valueMap)
	} else {
		return getValueAtPath(strings.Split(keyPath, "/"), valueMap)
	}
}

func getValueAtPath(keyPath []string, valueMap interface{}) (interface{}, error) {

	var (
		err error
		ok  bool
		i   int
		m   map[string]interface{}
		a   []interface{}
		v   interface{}
	)

	if m, ok = valueMap.(map[string]interface{}); !ok {
		if a, ok = valueMap.([]interface{}); !ok {
			return nil, fmt.Errorf("given value map object is not of type map[string]interface{} or []interface{}")
		}
		// value is an array
		if i, err = strconv.Atoi(keyPath[0]); err != nil {
			return nil, fmt.Errorf("array index was not an int")
		}
		if i >= len(a) {
			return nil, fmt.Errorf("array index greater than length of array %d", len(a))
		}
		v = a[i]

	} else {
		if v, ok = m[keyPath[0]]; !ok {
			// no value at given path so return nil
			return nil, nil
		}	
	}
	if len(keyPath) == 1 {
		return v, nil
	} else {
		// recursively lookup the value
		return getValueAtPath(keyPath[1:], v)
	}
}

// Sorts the given array of maps using the given key path as the sort key
//
// in: keyPath - key path where path separator is '/'
// in: valueMapArray - an array of maps to be sorted
func SortValueMap(keyPath string, valueMapArray interface{}) error {

	var (
		err error
		ok  bool

		p []string
		a []interface{}

		v1, v2 interface{}
		k1, k2 string
	)

	if a, ok = valueMapArray.([]interface{}); !ok {
		return fmt.Errorf("the provided value map array is not of type []interface{}")
	}

	err = nil
	p = strings.Split(keyPath, "/")

	compare := func(i, j int) bool {
		if err != nil {
			return false
		}
		if v1, err = getValueAtPath(p, a[i]); err != nil {
			return false
		}
		if v2, err = getValueAtPath(p, a[j]); err != nil {
			return false
		}
		if k1, ok = v1.(string); !ok {
			err = fmt.Errorf(
				"encountered a value '%#v' which was expected to be a string at key path '%s'",
				v1, keyPath)
			return false
		}
		if k2, ok = v2.(string); !ok {
			err = fmt.Errorf(
				"encountered a value '%#v' which was expected to be a string at key path '%s'",
				v1, keyPath)
			return false
		}
		return strings.Compare(k1, k2) == -1
	}
	sort.Slice(a, compare)
	return err
}
