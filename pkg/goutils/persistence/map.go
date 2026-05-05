package persistence

import (
	"fmt"
)

type Map map[string]interface{}

func NewMap() Map {
	return make(map[string]interface{})
}

func (m Map) Unmarshal(
	path []string,
	key string,
	elemType ElementType,
	value interface{},
) (
	Unmarshaller,
	Unmarshaller,
	error,
) {

	switch elemType {
	case EtObject:
		mm := NewMap()
		m[key] = mm
		return mm, m, nil

	case EtArray:
		aa := NewArray()
		m[key] = aa
		return aa, m, nil

	case EtValue:
		m[key] = value

	default:
		return nil, nil, fmt.Errorf("invalid type for Map container: %#v", elemType)
	}

	return m, m, nil
}

func (m Map) Finalize(
	path []string,
	key string,
	node Unmarshaller,
) error {

	var (
		ok bool
	)

	switch node.(type) {
	case Array:
		if _, ok = m[key]; !ok {
			return fmt.Errorf("attempt to finalize a non-existent key '%s' in object.", key)
		}
		m[key] = node
	}
	return nil
}
