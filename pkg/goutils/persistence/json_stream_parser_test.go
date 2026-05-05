package persistence_test

import (
	"strings"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/persistence"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("json stream parser tests", func() {

	var (
		err error
	)

	Context("parsing", func() {

		It("generates the correct sequence of events from a json stream", func() {

			type parseEvent struct {
				path     []string
				key      string
				elemType persistence.ElementType
				value    interface{}
			}

			type finalizeEvent struct {
				path []string
				key  string
			}

			events := []interface{}{
				parseEvent{path: []string{}, key: "nested_key_1", elemType: persistence.EtObject, value: nil},
				parseEvent{path: []string{"nested_key_1"}, key: "key1", elemType: persistence.EtValue, value: "value1"},
				parseEvent{path: []string{"nested_key_1"}, key: "array_key1", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"nested_key_1", "array_key1"}, key: "@0", elemType: persistence.EtArrayValue, value: "array_value11"},
				parseEvent{path: []string{"nested_key_1", "array_key1"}, key: "@1", elemType: persistence.EtArrayValue, value: "array_value12"},
				parseEvent{path: []string{"nested_key_1", "array_key1"}, key: "@2", elemType: persistence.EtArrayValue, value: "array_value13"},
				finalizeEvent{path: []string{"nested_key_1"}, key: "array_key1"},
				parseEvent{path: []string{"nested_key_1"}, key: "key2", elemType: persistence.EtValue, value: "value2"},
				parseEvent{path: []string{"nested_key_1"}, key: "nested_key_2", elemType: persistence.EtObject, value: nil},
				parseEvent{path: []string{"nested_key_1", "nested_key_2"}, key: "key3", elemType: persistence.EtValue, value: "value3"},
				parseEvent{path: []string{"nested_key_1", "nested_key_2"}, key: "key4", elemType: persistence.EtValue, value: "value4"},
				finalizeEvent{path: []string{"nested_key_1"}, key: "nested_key_2"},
				finalizeEvent{path: []string{}, key: "nested_key_1"},
				parseEvent{path: []string{}, key: "array_key2", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"array_key2"}, key: "@0", elemType: persistence.EtObject, value: nil},
				parseEvent{path: []string{"array_key2", "@0"}, key: "key5", elemType: persistence.EtValue, value: "value5"},
				parseEvent{path: []string{"array_key2", "@0"}, key: "array_key3", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"array_key2", "@0", "array_key3"}, key: "@0", elemType: persistence.EtArrayValue, value: float64(11)},
				parseEvent{path: []string{"array_key2", "@0", "array_key3"}, key: "@1", elemType: persistence.EtArrayValue, value: float64(22)},
				parseEvent{path: []string{"array_key2", "@0", "array_key3"}, key: "@2", elemType: persistence.EtArrayValue, value: float64(33)},
				finalizeEvent{path: []string{"array_key2", "@0"}, key: "array_key3"},
				parseEvent{path: []string{"array_key2", "@0"}, key: "key6", elemType: persistence.EtValue, value: "value6"},
				finalizeEvent{path: []string{"array_key2"}, key: "@0"},
				parseEvent{path: []string{"array_key2"}, key: "@1", elemType: persistence.EtObject, value: nil},
				parseEvent{path: []string{"array_key2", "@1"}, key: "key7", elemType: persistence.EtValue, value: "value7"},
				parseEvent{path: []string{"array_key2", "@1"}, key: "array_key4", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"array_key2", "@1", "array_key4"}, key: "@0", elemType: persistence.EtObject, value: nil},
				parseEvent{path: []string{"array_key2", "@1", "array_key4", "@0"}, key: "key8", elemType: persistence.EtValue, value: "value8"},
				parseEvent{path: []string{"array_key2", "@1", "array_key4", "@0"}, key: "key9", elemType: persistence.EtValue, value: "value9"},
				finalizeEvent{path: []string{"array_key2", "@1", "array_key4"}, key: "@0"},
				parseEvent{path: []string{"array_key2", "@1", "array_key4"}, key: "@1", elemType: persistence.EtArrayValue, value: "array_value14"},
				parseEvent{path: []string{"array_key2", "@1", "array_key4"}, key: "@2", elemType: persistence.EtObject, value: nil},
				parseEvent{path: []string{"array_key2", "@1", "array_key4", "@2"}, key: "key10", elemType: persistence.EtValue, value: "value10"},
				finalizeEvent{path: []string{"array_key2", "@1", "array_key4"}, key: "@2"},
				finalizeEvent{path: []string{"array_key2", "@1"}, key: "array_key4"},
				parseEvent{path: []string{"array_key2", "@1"}, key: "nested_key_3", elemType: persistence.EtObject, value: nil},
				parseEvent{path: []string{"array_key2", "@1", "nested_key_3"}, key: "key11", elemType: persistence.EtValue, value: "value11"},
				parseEvent{path: []string{"array_key2", "@1", "nested_key_3"}, key: "key12", elemType: persistence.EtValue, value: "value12"},
				parseEvent{path: []string{"array_key2", "@1", "nested_key_3"}, key: "key13", elemType: persistence.EtValue, value: "value13"},
				finalizeEvent{path: []string{"array_key2", "@1"}, key: "nested_key_3"},
				finalizeEvent{path: []string{"array_key2"}, key: "@1"},
				finalizeEvent{path: []string{}, key: "array_key2"},
				parseEvent{path: []string{}, key: "array_key5", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"array_key5"}, key: "@0", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"array_key5", "@0"}, key: "@0", elemType: persistence.EtArrayValue, value: float64(0)},
				parseEvent{path: []string{"array_key5", "@0"}, key: "@1", elemType: persistence.EtArrayValue, value: float64(1)},
				parseEvent{path: []string{"array_key5", "@0"}, key: "@2", elemType: persistence.EtArrayValue, value: float64(2)},
				finalizeEvent{path: []string{"array_key5"}, key: "@0"},
				parseEvent{path: []string{"array_key5"}, key: "@1", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"array_key5", "@1"}, key: "@0", elemType: persistence.EtArrayValue, value: float64(3)},
				parseEvent{path: []string{"array_key5", "@1"}, key: "@1", elemType: persistence.EtArrayValue, value: float64(4)},
				parseEvent{path: []string{"array_key5", "@1"}, key: "@2", elemType: persistence.EtArrayValue, value: float64(5)},
				parseEvent{path: []string{"array_key5", "@1"}, key: "@3", elemType: persistence.EtArrayValue, value: float64(6)},
				parseEvent{path: []string{"array_key5", "@1"}, key: "@4", elemType: persistence.EtArrayValue, value: float64(7)},
				finalizeEvent{path: []string{"array_key5"}, key: "@1"},
				parseEvent{path: []string{"array_key5"}, key: "@2", elemType: persistence.EtArray, value: nil},
				parseEvent{path: []string{"array_key5", "@2"}, key: "@0", elemType: persistence.EtArrayValue, value: float64(8)},
				parseEvent{path: []string{"array_key5", "@2"}, key: "@1", elemType: persistence.EtArrayValue, value: float64(9)},
				finalizeEvent{path: []string{"array_key5"}, key: "@2"},
				finalizeEvent{path: []string{}, key: "array_key5"},
				parseEvent{path: []string{}, key: "key14", elemType: persistence.EtValue, value: "value14"},
			}

			var (
				j jsonUnmarshallerTest

				counter, processedEvents int
			)

			counter = 0
			processedEvents = len(events)

			j = jsonUnmarshallerTest{

				unmarshal: func(
					path []string,
					key string,
					elemType persistence.ElementType,
					value interface{},
				) {

					e, ok := events[counter].(parseEvent)
					Expect(ok).To(BeTrue())
					Expect(path).To(Equal(e.path))
					Expect(key).To(Equal(e.key))
					Expect(elemType).To(Equal(e.elemType))

					if e.value == nil {
						Expect(value).To(BeNil())
					} else {
						Expect(value).To(Equal(e.value))
					}

					counter++
					processedEvents--
				},

				finalize: func(path []string, key string) {

					e, ok := events[counter].(finalizeEvent)
					Expect(ok).To(BeTrue())
					Expect(path).To(Equal(e.path))
					Expect(key).To(Equal(e.key))

					counter++
					processedEvents--
				},
			}

			parser := persistence.NewJSONStreamParser(j)
			_, err = parser.Parse(strings.NewReader(jsonDocument))
			Expect(err).ToNot(HaveOccurred())
			Expect(processedEvents).To(Equal(0))
		})

		It("unmarshalls a json stream to a map", func() {

			expectedData := persistence.Map{
				"nested_key_1": persistence.Map{
					"key1": "value1",
					"array_key1": persistence.Array{
						"array_value11",
						"array_value12",
						"array_value13",
					},
					"key2": "value2",
					"nested_key_2": persistence.Map{
						"key3": "value3",
						"key4": "value4",
					},
				},
				"array_key2": persistence.Array{
					persistence.Map{
						"key5": "value5",
						"array_key3": persistence.Array{
							float64(11),
							float64(22),
							float64(33),
						},
						"key6": "value6",
					},
					persistence.Map{
						"key7": "value7",
						"array_key4": persistence.Array{
							persistence.Map{
								"key8": "value8",
								"key9": "value9",
							},
							"array_value14",
							persistence.Map{
								"key10": "value10",
							},
						},
						"nested_key_3": persistence.Map{
							"key11": "value11",
							"key12": "value12",
							"key13": "value13",
						},
					},
				},
				"array_key5": persistence.Array{
					persistence.Array{
						float64(0),
						float64(1),
						float64(2),
					},
					persistence.Array{
						float64(3),
						float64(4),
						float64(5),
						float64(6),
						float64(7),
					},
					persistence.Array{
						float64(8),
						float64(9),
					},
				},
				"key14": "value14",
			}

			var (
				err              error
				unmarshalledData persistence.Unmarshaller
			)

			parser := persistence.NewJSONStreamParser(persistence.NewMap())
			unmarshalledData, err = parser.Parse(strings.NewReader(jsonDocument))
			Expect(err).ToNot(HaveOccurred())
			logger.DebugMessage("unmarshalled data: %# v", unmarshalledData)

			Expect(unmarshalledData).To(Equal(expectedData))
		})
	})
})

type jsonUnmarshallerTest struct {
	unmarshal func([]string, string, persistence.ElementType, interface{})
	finalize  func([]string, string)
}

func (j jsonUnmarshallerTest) Unmarshal(
	path []string,
	key string,
	elemType persistence.ElementType,
	value interface{},
) (
	persistence.Unmarshaller,
	persistence.Unmarshaller,
	error,
) {

	logger.DebugMessage("unmarshal => path: %#v, key: \"%s\", elemType: %#v, value: %#v\n",
		path, key, elemType, value)
	j.unmarshal(path, key, elemType, value)
	return j, j, nil
}

func (j jsonUnmarshallerTest) Finalize(
	path []string,
	key string,
	node persistence.Unmarshaller,
) error {

	logger.DebugMessage("finalize => path: %#v, key: \"%s\"\n", path, key)
	j.finalize(path, key)
	return nil
}

const jsonDocument = `
{
	"nested_key_1": {
		"key1": "value1",
		"array_key1": [ 
			"array_value11", 
			"array_value12", 
			"array_value13"
		],
		"key2": "value2",
		"nested_key_2": {
			"key3": "value3",
			"key4": "value4"
		}
	},
	"array_key2": [
		{
			"key5": "value5",
			"array_key3": [ 11, 22, 33 ],
			"key6": "value6"
		},
		{
			"key7": "value7",
			"array_key4": [
				{
					"key8": "value8",
					"key9": "value9"
				},
				"array_value14",
				{
					"key10": "value10"
				}
			],
			"nested_key_3": {
				"key11": "value11",
				"key12": "value12",
				"key13": "value13"
			}
		}
	],
	"array_key5": [
		[ 0, 1, 2 ],
		[ 3, 4, 5, 6, 7 ],
		[ 8, 9 ]
	],
	"key14": "value14"
}
`
