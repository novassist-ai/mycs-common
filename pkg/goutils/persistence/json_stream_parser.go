package persistence

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

type JSONStreamParser struct {
	root Unmarshaller
}

func NewJSONStreamParser(u Unmarshaller) *JSONStreamParser {

	return &JSONStreamParser{
		root: u,
	}
}

func (p *JSONStreamParser) Parse(input io.Reader) (Unmarshaller, error) {

	// function to determine type of current parsed json element
	typeFromToken := func(prevType ElementType, token json.Token) (ElementType, error) {

		switch tokenType := token.(type) {

		case json.Delim:

			switch token.(json.Delim).String() {
			case "{":
				return EtObject, nil

			case "[":
				return EtArray, nil

			case "}", "]":
				return EtUnknown, nil

			default:
				return EtUnknown, fmt.Errorf("unrecognized token type '%v': %s",
					tokenType, token.(json.Delim).String())
			}

		case string, float64, bool:

			switch prevType {
			case EtKey:
				return EtValue, nil

			case EtValue:
				return EtKey, nil

			case EtArray:
				return EtArrayValue, nil

			case EtArrayValue:
				return EtArrayValue, nil

			default: /* EtObject */
				if reflect.ValueOf(token).Kind() == reflect.String {
					return EtKey, nil
				} else {
					return EtUnknown, fmt.Errorf("key was not of type string")
				}
			}

		default:
			return EtUnknown, nil
		}
	}

	var (
		err error

		token json.Token

		keyPathLastIdx, umStackLastIdx     int
		currType, prevType                 ElementType
		currUnmarshaller, nextUnmarshaller Unmarshaller

		lastKey        string
		lastArrayIndex int
	)

	decoder := json.NewDecoder(input)
	keyPath := []string{""}
	umStack := []Unmarshaller{p.root}
	lastArrayIndex = -1

	if token, err = decoder.Token(); err != nil {
		if err == io.EOF {
			return umStack[0], nil
		} else {
			return nil, err
		}
	}
	if prevType, err = typeFromToken(EtUnknown, token); err != nil {
		return nil, err
	}

	for {
		if token, err = decoder.Token(); err != nil {
			if err == io.EOF {
				return currUnmarshaller, nil
			} else {
				return nil, err
			}
		}

		keyPathLastIdx = len(keyPath) - 1
		umStackLastIdx = len(umStack) - 1

		lastKey = keyPath[keyPathLastIdx]

		currUnmarshaller = umStack[umStackLastIdx]
		if currType, err = typeFromToken(prevType, token); err != nil {
			return nil, err
		}

		if currType == EtKey {
			keyPath = append(keyPath, token.(string))
			lastArrayIndex = -1

		} else if currType == EtUnknown {

			// pop last node off stack as we reached
			// the end of a json object or array
			umStack = umStack[:umStackLastIdx]
			keyPath = keyPath[:keyPathLastIdx]

			if umStackLastIdx > 0 {
				if err = umStack[umStackLastIdx-1].Finalize(keyPath[1:keyPathLastIdx], lastKey, currUnmarshaller); err != nil {
					return nil, err
				}
			}

			if len(lastKey) > 0 && lastKey[0] == '@' {
				lastArrayIndex, _ = strconv.Atoi(lastKey[1:])
				currType = EtArrayValue
			} else {
				lastArrayIndex = -1
			}

		} else /* EtObject/EtArray/Value */ {

			switch currType {

			case EtObject, EtArray:
				if len(umStack) == len(keyPath) {
					// array elements do not have a key
					// so push an implicit placeholder key
					lastKey = fmt.Sprintf("@%d", lastArrayIndex+1)
					keyPath = append(keyPath, lastKey)
					keyPathLastIdx++
				}

				nextUnmarshaller, umStack[umStackLastIdx], err = currUnmarshaller.Unmarshal(
					keyPath[1:keyPathLastIdx], lastKey, currType, nil)
				umStack = append(umStack, nextUnmarshaller)

			case EtValue:
				_, umStack[umStackLastIdx], err = currUnmarshaller.Unmarshal(
					keyPath[1:keyPathLastIdx], lastKey, currType, token)
				keyPath = keyPath[:keyPathLastIdx]

			default: /* EtArrayValue */
				lastArrayIndex++
				_, umStack[umStackLastIdx], err = currUnmarshaller.Unmarshal(
					keyPath[1:], fmt.Sprintf("@%d", lastArrayIndex), currType, token)
			}
			if err != nil {
				return nil, err
			}
			if currType != EtArrayValue {
				lastArrayIndex = -1
			}
		}
		prevType = currType
	}
}
