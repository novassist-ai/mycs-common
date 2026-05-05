package run

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/streams"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
)

type CLI interface {
	ExecutablePath() string
	WorkingDirectory() string

	ApplyFilter(filter *streams.Filter)
	GetPipedOutputBuffer() io.Reader
	GetPipedErrorBuffer() io.Reader

	Run(args []string) error
	RunWithEnv(args []string, extraEnvVars []string) error

	Start(args []string) error
	StartWithEnv(args []string, extraEnvVars []string) error

	Wait(timeout... time.Duration) error
	Stop(timeout... time.Duration) error
}

type cli struct {
	executablePath            string
	workingDirectory          string
	outputBuffer, errorBuffer io.Writer

	// Original buffer if pipe was created
	outBuffer, errBuffer         io.Writer
	outPipeWriter, errPipeWriter *io.PipeWriter

	// Original buffer after filter has been applied
	outFilteredWriter   io.WriteCloser
	outUnfilteredBuffer io.Writer
	filteredAll         bool

	// running process
	command *exec.Cmd
}

func NewCLI(
	executablePath string,
	workingDirectory string,
	outputBuffer, errorBuffer io.Writer,
) (CLI, error) {

	var (
		err  error
		info os.FileInfo
	)

	logger.TraceMessage("cli.NewCLI(): Creating CLI to execute binary '%s' from path '%s'.",
		executablePath,
		workingDirectory,
	)

	info, err = os.Stat(executablePath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("binary not found at '%s'", executablePath)
	}
	if err != nil {
		return nil, err
	}
	if (runtime.GOOS == "windows" && !strings.HasSuffix(executablePath, ".exe")) ||
		(runtime.GOOS != "windows" && (info.Mode()&0111) == 0) {
		return nil, fmt.Errorf("binary at '%s' is not executable", executablePath)
	}

	info, err = os.Stat(workingDirectory)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("working directory not found at '%s'", workingDirectory)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("working directory '%s' is not a directory", workingDirectory)
	}

	return &cli{
		executablePath:   executablePath,
		workingDirectory: workingDirectory,

		outputBuffer: outputBuffer,
		errorBuffer:  errorBuffer,

		outPipeWriter: nil,
		errPipeWriter: nil,

		outBuffer: nil,
		errBuffer: nil,
	}, nil
}

func (c *cli) ExecutablePath() string {
	return c.executablePath
}

func (c *cli) WorkingDirectory() string {
	return c.workingDirectory
}

func (c *cli) ApplyFilter(filter *streams.Filter) {

	if c.outUnfilteredBuffer != nil {
		panic("a filter can only be applied once")
	}

	// flags if filter is being applied on top
	// of an existing output aggregator
	c.filteredAll = (c.outPipeWriter != nil)

	c.outUnfilteredBuffer = c.outputBuffer
	c.outFilteredWriter = streams.NewFilterWriter(filter, c.outUnfilteredBuffer)
	c.outputBuffer = c.outFilteredWriter
}

func (c *cli) GetPipedOutputBuffer() io.Reader {

	if c.outPipeWriter != nil {
		panic("you can retrieve a piped output buffer only once")
	}

	// save original buffer
	c.outBuffer = c.outputBuffer

	pr, pw := io.Pipe()
	c.outPipeWriter = pw
	c.outputBuffer = io.MultiWriter(c.outBuffer, c.outPipeWriter)
	return pr
}

func (c *cli) GetPipedErrorBuffer() io.Reader {

	if c.errPipeWriter != nil {
		panic("you can retrieve a piped error buffer only once")
	}

	// save original buffer
	c.errBuffer = c.errorBuffer

	pr, pw := io.Pipe()
	c.errPipeWriter = pw
	c.errorBuffer = io.MultiWriter(c.errBuffer, c.errPipeWriter)
	return pr
}

func (c *cli) Run(
	args []string,
) error {
	return c.RunWithEnv(args, []string{})
}

func (c *cli) RunWithEnv(
	args []string,
	extraEnvVars []string,
) error {

	var (
		err error
	)
	if err = c.StartWithEnv(args, extraEnvVars); err != nil {
		return err
	}
	return c.Wait(0)
}

func (c *cli) Start(
	args []string,
) error {
	return c.StartWithEnv(args, []string{})
}

func (c *cli) StartWithEnv(
	args []string,
	extraEnvVars []string,
) error {

	c.command = exec.Command(c.executablePath, args...)
	c.command.Dir = c.workingDirectory

	c.command.Env = os.Environ()
	c.command.Env = append(c.command.Env, extraEnvVars...)

	c.command.Stdout = c.outputBuffer
	c.command.Stderr = c.errorBuffer

	logger.TraceMessage("cli.Start(): CLI command environment: %# v", c.command.Env)
	logger.TraceMessage("cli.Start(): CLI command working dir: %s", c.workingDirectory)
	logger.TraceMessage("cli.Start(): Starting CLI command: %s %s", c.executablePath, strings.Join(args, " "))

	return c.command.Start()
}

func (c *cli) Wait(timeout... time.Duration) error {

	var (
		err error

		runTimeout, stopTimeout time.Duration
	)

	if c.command != nil {

		if len(timeout) > 0 {
			runTimeout = timeout[0]
		} else {
			runTimeout = 0
		}
		if len(timeout) > 1 {
			stopTimeout = timeout[1]
		} else {
			stopTimeout = 100 * time.Millisecond
		}
	
		logger.TraceMessage("cli.Wait(): Waiting for CLI command to finish: %s", c.executablePath)	
		if utils.InvokeWithTimeout(
			func() {
				err = c.command.Wait()
			},
			runTimeout,
		) {
			c.cleanUp()
		} else {
			logger.TraceMessage("cli.Wait(): CLI command timedout after %d seconds: %s", runTimeout / time.Second, c.executablePath)
			err = c.Stop(stopTimeout)
		}
	}
	return err
}

func (c *cli) Stop(timeout... time.Duration) error {

	var (
		err error

		stopTimeout time.Duration
	)

	if c.command != nil {

		if len(timeout) > 0 {
			stopTimeout = timeout[0]
		} else {
			stopTimeout = 0
		}

		logger.TraceMessage("cli.Stop(): Signalling CLI command to stop: %s", c.executablePath)	
		if err = c.command.Process.Signal(os.Interrupt); err != nil {
			return err
		}

		if !utils.InvokeWithTimeout(
			func() {				
				_, err = c.command.Process.Wait()
			},
			stopTimeout,
		) {
			logger.TraceMessage("cli.Stop(): CLI command did not stop after %d seconds. Process will be killed: %s", stopTimeout / time.Second, c.executablePath)
			err = c.command.Process.Kill()

			if !utils.InvokeWithTimeout(
				func() {				
					_, err = c.command.Process.Wait()
				},
				100 * time.Millisecond,
			) {
				logger.TraceMessage("cli.Stop(): CLI command could not be killed: %s", c.executablePath)
			}
		}
		c.cleanUp()
	}
	return err
}

func (c *cli) cleanUp() {

	// Restore buffers if piped
	if c.outBuffer != nil {
		c.outPipeWriter.Close()

		if c.outUnfilteredBuffer != nil {
			// filter writer needs to be close to
			// flush any unwritten data
			c.outFilteredWriter.Close()

			if !c.filteredAll {
				// Discard the filtered buffer passed
				// to the call to io.MultiWriter
				c.outBuffer = c.outUnfilteredBuffer

				// if all buffers have not been filtered
				// then c.outUnfilteredBuffer will be the
				// writer created by io.MultiWriter which
				// can be discarded.
			}
		}
		c.outputBuffer = c.outBuffer

	} else if c.outUnfilteredBuffer != nil {
		// filter writer needs to be close to
		// flush any unwritten data
		c.outFilteredWriter.Close()
		c.outputBuffer = c.outUnfilteredBuffer
	}
	if c.errBuffer != nil {
		c.errPipeWriter.Close()
		c.errorBuffer = c.errBuffer
	}

	// reset filters and pipes
	c.outPipeWriter = nil
	c.outBuffer = nil
	c.errPipeWriter = nil
	c.errBuffer = nil
	c.outFilteredWriter = nil
	c.outUnfilteredBuffer = nil
	c.filteredAll = false
	c.command = nil
}
