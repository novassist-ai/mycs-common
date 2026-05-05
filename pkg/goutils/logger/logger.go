package logger

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/kr/pretty"
	log "github.com/sirupsen/logrus"
)

func Initialize() {

	switch logLevel := os.Getenv("CBS_LOGLEVEL"); logLevel {
	case "trace":
		SetConsoleLogger(log.TraceLevel)
	case "debug":
		SetConsoleLogger(log.DebugLevel)
	case "info":
		SetConsoleLogger(log.InfoLevel)
	case "warn":
		SetConsoleLogger(log.WarnLevel)
	case "fatal":
		SetConsoleLogger(log.FatalLevel)
	default:
		SetConsoleLogger(log.ErrorLevel)
	}
}

func SetConsoleLogger(level log.Level) {

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	log.SetLevel(level)
}

func TraceMessage(format string, v ...interface{}) {
	if log.IsLevelEnabled(log.TraceLevel) {
		logMultiLine(fmt.Sprintf(format, preFormatArgs(v)...), log.Trace)
	}
}

func DebugMessage(format string, v ...interface{}) {
	if log.IsLevelEnabled(log.DebugLevel) {
		logMultiLine(fmt.Sprintf(format, preFormatArgs(v)...), log.Debug)
	}
}

func InfoMessage(format string, v ...interface{}) {
	if log.IsLevelEnabled(log.InfoLevel) {
		logMultiLine(fmt.Sprintf(format, preFormatArgs(v)...), log.Info)
	}
}

func WarnMessage(format string, v ...interface{}) {
	if log.IsLevelEnabled(log.WarnLevel) {
		logMultiLine(fmt.Sprintf(format, preFormatArgs(v)...), log.Warn)
	}
}

func ErrorMessage(format string, v ...interface{}) {
	if log.IsLevelEnabled(log.ErrorLevel) {
		logMultiLine(fmt.Sprintf(format, preFormatArgs(v)...), log.Error)
	}
}

func preFormatArgs(v []interface{}) []interface{} {
	vv := []interface{}{}
	for _, o := range v {
		k := reflect.ValueOf(o).Kind()
		if k == reflect.Struct ||
			k == reflect.Interface ||
			k == reflect.Ptr ||
			k == reflect.Slice ||
			k == reflect.Array ||
			k == reflect.Map {
			vv = append(vv, pretty.Formatter(o))
		} else {
			vv = append(vv, o)
		}
	}
	return vv
}

func logMultiLine(
	message string,
	logFunc func(args ...interface{}),
) {

	now := time.Now()
	s := bufio.NewScanner(strings.NewReader(message))
	for s.Scan() {
		log.WithTime(now)
		logFunc(s.Text())
	}
}
