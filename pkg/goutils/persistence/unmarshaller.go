package persistence

type ElementType int

const (
	EtUnknown ElementType = iota
	EtObject
	EtArray
	EtKey
	EtValue
	EtArrayValue
)

type Unmarshaller interface {

	// unmarshall callback function to handle parsed elements.
	//
	// in: path     - the absolute parent path of the child element being handled
	// in: key      - the key of the stream element being handled
	// in: elemType - the type of the stream element. this can be one of Object, Array
	//                or Value. if it is an Object or Array then the returned
	//                Unmarshaller can be specific for that instance
	// in: value    - the value to be handled. if the key type is an Object
	//                or Array this will be nil
	//
	// out: Unmarshaller - the unmarshaller for current node
	// out: Unmarshaller - the unmarshaller for the next node
	// out: error        - if an error occurs while handling the data
	Unmarshal(
		path []string,
		key string,
		elemType ElementType,
		value interface{},
	) (Unmarshaller, Unmarshaller, error)

	// finalizes the a stream object or array once it has been completely parsed
	//
	// in: path - the absolute parent path of the child element being handled
	// in: key  - the key of the stream element being handled
	// in: node - the node to be finalized
	//
	// out: error - if an error occurs while handling the data
	Finalize(
		path []string,
		key string,
		node Unmarshaller,
	) error
}
