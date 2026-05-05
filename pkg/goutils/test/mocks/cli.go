package mocks

import (
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/streams"
	. "github.com/onsi/gomega"
)

type fakeResponse struct {
	outResponse string
	errResponse string
	err         error
}

type FakeCLI struct {
	outputBuffer, errorBuffer io.Writer

	outBuffer, errBuffer         io.Writer
	outPipeWriter, errPipeWriter *io.PipeWriter

	FuncRunCounter, FuncRunWithEnvCounter int

	expectRequestKeys []string
	fakeResponses     map[string]fakeResponse
}

func NewFakeCLI(outputBuffer, errorBuffer io.Writer) *FakeCLI {

	fakeCli := &FakeCLI{
		outputBuffer: outputBuffer,
		errorBuffer:  errorBuffer,

		outBuffer: nil,
		errBuffer: nil,

		outPipeWriter: nil,
		errPipeWriter: nil,

		FuncRunCounter:        0,
		FuncRunWithEnvCounter: 0,

		expectRequestKeys: []string{},
		fakeResponses:     make(map[string]fakeResponse),
	}

	return fakeCli
}

func (c *FakeCLI) Reset() {
	c.FuncRunCounter = 0
	c.FuncRunWithEnvCounter = 0
	c.expectRequestKeys = []string{}
}

func (c *FakeCLI) AddFakeResponse(
	inArgs []string,
	inExtraEnvVars []string,
	outResponse string,
	errResponse string,
	err error,
) string {

	requestKey := createRequestKey(inArgs, inExtraEnvVars)
	c.fakeResponses[requestKey] = fakeResponse{
		outResponse: outResponse,
		errResponse: errResponse,
		err:         err,
	}

	return requestKey
}

func (c *FakeCLI) IsExpectedRequestStackEmpty() bool {
	return len(c.expectRequestKeys) == 0
}

func (c *FakeCLI) ExpectFakeRequest(key string) {
	// Add expected request
	c.expectRequestKeys = append(c.expectRequestKeys, key)
}

func (c *FakeCLI) ExecutablePath() string {
	return "/goutils/test/cli/executable"
}

func (c *FakeCLI) WorkingDirectory() string {
	return "/goutils/test/cli/workingdirectory"
}

func (c *FakeCLI) ApplyFilter(filter *streams.Filter) {
}

func (c *FakeCLI) GetPipedOutputBuffer() io.Reader {
	c.outBuffer = c.outputBuffer

	pr, pw := io.Pipe()
	c.outPipeWriter = pw
	c.outputBuffer = io.MultiWriter(c.outBuffer, c.outPipeWriter)
	return pr
}

func (c *FakeCLI) GetPipedErrorBuffer() io.Reader {
	c.errBuffer = c.errorBuffer

	pr, pw := io.Pipe()
	c.errPipeWriter = pw
	c.errorBuffer = io.MultiWriter(c.errBuffer, c.errPipeWriter)
	return pr
}

func (c *FakeCLI) Run(
	args []string,
) error {
	c.FuncRunCounter++
	return c.RunWithEnv(args, []string{})
}

func (c *FakeCLI) RunWithEnv(
	args []string,
	extraEnvVars []string,
) error {

	var err error

	c.FuncRunWithEnvCounter++
	requestKey := createRequestKey(args, extraEnvVars)

	l := len(c.expectRequestKeys)
	if l > 0 {
		// Remove expected request from top of list
		// and verify current request against it
		expectRequestKey := c.expectRequestKeys[0]
		c.expectRequestKeys = c.expectRequestKeys[1:l]

		if debug {
			log.Printf("DEBUG: given request = %s\n", requestKey)
			log.Printf("DEBUG: expected request = %s\n\n", expectRequestKey)
		}

		Expect(requestKey).To(Equal(expectRequestKey))
	}

	if r, ok := c.fakeResponses[requestKey]; ok {
		// Return fake response matching request

		_, err = c.outputBuffer.Write([]byte(r.outResponse))
		Expect(err).NotTo(HaveOccurred())
		if c.outBuffer != nil {
			c.outPipeWriter.Close()
			c.outPipeWriter = nil

			c.outputBuffer = c.outBuffer
			c.outBuffer = nil
		}

		_, err = c.errorBuffer.Write([]byte(r.errResponse))
		Expect(err).NotTo(HaveOccurred())
		if c.errBuffer != nil {
			c.errPipeWriter.Close()
			c.errPipeWriter = nil

			c.errorBuffer = c.errBuffer
			c.errBuffer = nil
		}

		return r.err
	}

	return fmt.Errorf("Fake response not found for key '%s'", requestKey)
}

func (c *FakeCLI) Start(args []string) error {
	return c.Run(args)
}

func (c *FakeCLI) StartWithEnv(args []string, extraEnvVars []string) error {
	return c.RunWithEnv(args, extraEnvVars)
}

func (c *FakeCLI) Wait(timeout... time.Duration) error {
	return nil
}

func (c *FakeCLI) Stop(timeout... time.Duration) error {
	return nil
}

func createRequestKey(args []string, env []string) string {
	var key strings.Builder

	argList := make([]string, len(args))
	copy(argList, args)
	sort.Strings(argList)

	envList := make([]string, len(env))
	copy(envList, env)
	sort.Strings(envList)

	key.WriteString(strings.Join(argList, "&"))
	key.WriteString("++")
	key.WriteString(strings.Join(envList, "&"))
	return key.String()
}
