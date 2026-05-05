package mocks

import "os"

var debug bool

func Init() {
	if os.Getenv("CBS_LOGLEVEL") == "trace" {
		debug = true
	} else {
		debug = false
	}
}
