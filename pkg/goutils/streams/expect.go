package streams

import (
	"bytes"
	"io"
	"regexp"
	"sync"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

// Call back to close sessions
// and streams being intercepted
type EndSessionHandler func()

type ExpectStream struct {
	inputStream  io.Reader // input stream for data from client
	outputStream io.Writer // outut stream for data to client

	pipedReadStream  io.ReadCloser  // piped input for data to service
	pipedWriteStream io.WriteCloser // piped output for data from service

	streamReader io.ReadCloser  // stream on which to match expact patterns
	streamWriter io.WriteCloser // stream to which expect commands will be written

	// triggers on service's output stream
	serialExpects     []*Expect
	triggerOutExpects []*Expect
	// triggers on sender's input stream
	triggerInExpects []*Expect

	// internal buffer size (default 512)
	bufferSize int

	// internal error
	err error

	// syncs expect intercept thread
	expectSync sync.WaitGroup
	running    bool

	endSession EndSessionHandler
}

type Expect struct {
	// start of match
	StartPattern,
	// end of match on different line
	// - skip if match is on same line
	EndPattern string

	startPattern, endPattern *regexp.Regexp

	// command to send on match
	Command string

	// match only once line is complete
	OnNewLine bool
	// exit after sending command if matched
	Exit bool

	active bool
}

// the expect stream will search for expect patterns
// in the input stream and respond with commands in
// the output stream with a response associated
// with that pattern. patterns and their commands
// are evaluated in order.
func NewExpectStream(
	inputStream io.Reader, // i.e. standard in of client
	outputStream io.Writer, // i.e. standard out of client
	endSession EndSessionHandler,
) (
	*ExpectStream,
	io.ReadCloser, // i.e. replaces standard in for data to service
	io.WriteCloser, // i.e. replaces standard out for data from service
) {

	es := &ExpectStream{
		inputStream:  inputStream,
		outputStream: outputStream,

		serialExpects:     []*Expect{},
		triggerOutExpects: []*Expect{},

		triggerInExpects: []*Expect{},

		bufferSize: 512,
		running:    false,

		endSession: endSession,
	}

	es.pipedReadStream, es.streamWriter = io.Pipe()
	es.streamReader, es.pipedWriteStream = io.Pipe()

	return es, es.pipedReadStream, es.pipedWriteStream
}

func (es *ExpectStream) SetBufferSize(size int) {
	es.bufferSize = size
}

func (es *ExpectStream) AddExpectOutTrigger(
	expect *Expect,
	serial bool,
) {
	if len(expect.StartPattern) > 0 {
		expect.startPattern = regexp.MustCompile(expect.StartPattern)
	}
	if len(expect.EndPattern) > 0 {
		expect.endPattern = regexp.MustCompile(expect.EndPattern)
	}
	if serial {
		es.serialExpects = append(es.serialExpects, expect)
	} else {
		es.triggerOutExpects = append(es.triggerOutExpects, expect)
	}
}

func (es *ExpectStream) AddExpectInTrigger(
	expect *Expect,
) {
	if len(expect.StartPattern) > 0 {
		expect.startPattern = regexp.MustCompile(expect.StartPattern)
	}
	if len(expect.EndPattern) > 0 {
		expect.endPattern = regexp.MustCompile(expect.EndPattern)
	}
	es.triggerInExpects = append(es.triggerInExpects, expect)
}

func (es *ExpectStream) SetShellExitCommand(command string) {

	es.triggerInExpects = append(es.triggerInExpects,
		&Expect{
			startPattern: regexp.MustCompile(`\x04`),
			endPattern:   nil,
			Command:      command,
			OnNewLine:    false,
			Exit:         true,
			active:       false,
		},
	)
	es.triggerInExpects = append(es.triggerInExpects,
		&Expect{
			startPattern: regexp.MustCompile(`^exit$`),
			endPattern:   nil,
			Command:      command,
			OnNewLine:    true,
			Exit:         true,
			active:       false,
		},
	)
	es.triggerInExpects = append(es.triggerInExpects,
		&Expect{
			startPattern: regexp.MustCompile(`^logout$`),
			endPattern:   nil,
			Command:      command,
			OnNewLine:    true,
			Exit:         true,
			active:       false,
		},
	)
}

func (es *ExpectStream) Start() {

	es.running = true
	es.expectSync.Add(2)

	// Handle stream output from service
	go es.startStreamHandler(es.streamReader, es.outputStream, es.processOutExpects)
	// Handle stream input from sender
	go es.startStreamHandler(es.inputStream, es.streamWriter, es.processInExpects)
}

func (es *ExpectStream) startStreamHandler(
	expectStreamReader io.Reader,
	echoStreamWriter io.Writer,
	expectHandler func([]byte, bool) error,
) {

	var (
		err error

		i, j, l int
		newLine bool

		// lenLastInputLine int

		lineBuffer bytes.Buffer
		line       []byte
	)

	lineBuffer.Grow(es.bufferSize)
	buffer := make([]byte, es.bufferSize)

	for err == nil && es.running {
		// read until a newline is encountered in bytes read
		if l, err = expectStreamReader.Read(buffer); err != nil && err != io.EOF {
			break
		}
		if _, err = echoStreamWriter.Write(buffer[0:l]); err != nil {
			break
		}

		for i = 0; i < l; {
			newLine = false
			for j = i; j < l; j++ {
				if buffer[j] == 10 || buffer[j] == 13 {
					newLine = true
					break
				}
			}

			lineBuffer.Write(buffer[i:j])
			line = lineBuffer.Bytes()

			if err = expectHandler(line, newLine); err != nil {
				break
			}

			if newLine {
				// if new line then we reset the line
				// buffer and start building a new line
				lineBuffer.Reset()
				i = j + 1
			} else {
				i = j
			}
		}
	}
	if err != io.EOF {
		es.err = err
	}

	if es.endSession != nil && expectStreamReader == es.streamReader {
		// If client's input terminated then call session end handler
		es.endSession()
	}
	es.expectSync.Done()
}

func (es *ExpectStream) processExpect(
	expect *Expect,
	line []byte,
	newLine bool,
) (bool, error) {

	var (
		err error
	)

	if !expect.OnNewLine || newLine {

		if !expect.active {
			// match start pattern of expect
			if expect.startPattern.Match(line) {
				expect.active = true
			}
		}
		if expect.active {
			// match end pattern of expect if one exists
			if expect.endPattern == nil ||
				expect.endPattern.Match(line) {

				logger.TraceMessage(
					"Matched expect pattern '%s' on stream '%s'.",
					expect.startPattern, line,
				)

				// if exit command then state of expect handler
				// running should be set to NOT running
				es.running = !expect.Exit

				// send command to reciever
				if len(expect.Command) > 0 {
					if _, err = es.streamWriter.Write([]byte(expect.Command)); err == nil {
						return true, nil
					}
				}
			}
		}
	}

	return false, err
}

func (es *ExpectStream) StartAsShell() {
	es.SetShellExitCommand("")
	es.Start()
}

func (es *ExpectStream) processOutExpects(
	line []byte,
	newLine bool,
) error {

	var (
		err error

		expect *Expect
		match  bool
	)

	// evaluate top of serial expects list popping
	// it and proceeding to the next once a match
	// has been made and executed
	if len(es.serialExpects) > 0 {
		expect = es.serialExpects[0]
		if match, err = es.processExpect(expect, line, newLine); err != nil {
			return err
		}
		if match {
			es.serialExpects = es.serialExpects[1:]
		}
	}

	// trigger expects are executed whenever a
	// match is made
	for _, expect = range es.triggerOutExpects {
		if _, err = es.processExpect(expect, line, newLine); err != nil {
			return err
		}
	}
	return nil
}

func (es *ExpectStream) processInExpects(
	line []byte,
	newLine bool,
) error {

	var (
		err error

		expect *Expect
	)

	// trigger expects are executed whenever a
	// match is made
	for _, expect = range es.triggerInExpects {
		if _, err = es.processExpect(expect, line, newLine); err != nil {
			return err
		}
	}
	return nil
}

func (es *ExpectStream) Close() {
	es.running = false

	// closing pipes write end should
	// close read end as well
	es.pipedWriteStream.Close()
	es.streamWriter.Close()
	es.streamReader.Close()
	es.pipedReadStream.Close()

	// wait until expect intercept
	// thread completes
	es.expectSync.Wait()
}
