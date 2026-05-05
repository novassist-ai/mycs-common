package utils

import (
	"encoding/json"
	"fmt"
)

var (
	JsonObjectStartDelim = json.Delim('{')
	JsonObjectEndDelim   = json.Delim('}')
	JsonArrayStartDelim  = json.Delim('[')
	JsonArrayEndDelim    = json.Delim(']')
)

// Reads a specified JSON delimiter from a json stream decoder.
// If the expected delimiter is not read then an error is returned.
//
// in: decoder - an instance of a json decoder stream reader
// in: delimiter - the delimiter expected to be read as the next token
// out: the json token read
func ReadJSONDelimiter(decoder *json.Decoder, delimiter json.Delim) (json.Token, error) {

	var (
		err   error
		token json.Token
	)

	if token, err = decoder.Token(); err != nil {
		return token, err
	}
	err = fmt.Errorf(
		"expected variable array delimiter '%s' not found",
		delimiter)

	switch t := token.(type) {
	case json.Delim:
		if t == delimiter {
			err = nil
		}
	default:
	}
	return token, err
}
