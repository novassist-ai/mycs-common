package run_test

import (
	"io"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/run"
	"github.com/novassist/mycs-common/pkg/goutils/streams"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CLI", func() {

	var (
		err error
		cli run.CLI

		outputBuffer, errorBuffer strings.Builder

		workingDirectory string
	)

	BeforeEach(func() {
		outputBuffer.Reset()
		errorBuffer.Reset()

		_, filename, _, _ := runtime.Caller(0)
		workingDirectory = path.Dir(filename)
	})

	Context("check CLI initialization errors", func() {

		var (
			nonExecutableFile string
		)

		BeforeEach(func() {
			nonExecutableFile = workingDirectory + "/cli.go"
		})

		It("CLI path not found error", func() {
			_, err = run.NewCLI("/usr/bin/i-do-not-exist", "/tmp", &outputBuffer, &errorBuffer)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(BeEquivalentTo("binary not found at '/usr/bin/i-do-not-exist'"))
		})

		It("CLI is not an executable error", func() {
			_, err = run.NewCLI(nonExecutableFile, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(BeEquivalentTo("binary at '" + nonExecutableFile + "' is not executable"))
		})

		It("invalid working directory path error", func() {
			_, err = run.NewCLI("/usr/bin/env", "/i-do-not-exist", &outputBuffer, &errorBuffer)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(BeEquivalentTo("working directory not found at '/i-do-not-exist'"))
		})

		It("invalid working directory path error", func() {
			_, err = run.NewCLI("/usr/bin/env", nonExecutableFile, &outputBuffer, &errorBuffer)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(BeEquivalentTo("working directory '" + nonExecutableFile + "' is not a directory"))
		})
	})

	Context("run shell cli command", func() {

		const (
			which = "/usr/bin/which"
			env   = "/usr/bin/env"
		)
		var (
			echoOutput string
		)

		BeforeEach(func() {
			echoOutput, err = filepath.Abs(workingDirectory + "/../test/fixtures/cli/echo-output.sh")
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs cli which returns an error", func() {
			cli, err = run.NewCLI(which, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.Run([]string{})
			Expect(err).To(HaveOccurred())
		})

		It("runs cli with and arg", func() {
			cli, err = run.NewCLI(which, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.Run([]string{"env"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Trim(outputBuffer.String(), "\r\n")).To(BeEquivalentTo(env))
		})

		It("runs cli with and validates environment was passed", func() {
			cli, err = run.NewCLI(env, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.Run([]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Trim(outputBuffer.String(), "\r\n")).To(MatchRegexp("\\n?PATH=.*\\n?"))
		})

		It("runs cli and captures output written stdout and stderr", func() {
			cli, err = run.NewCLI(echoOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.Run([]string{"aa", "bb"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Trim(outputBuffer.String(), "\r\n")).To(BeEquivalentTo("aa - written to stdout"))
			Expect(strings.Trim(errorBuffer.String(), "\r\n")).To(BeEquivalentTo("bb - written to stderr"))
		})

		It("runs cli and captures output written stdout and stderr as well as piped buffers", func() {
			cli, err = run.NewCLI(echoOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			var pipedOutputString strings.Builder
			pipedOutput := cli.GetPipedOutputBuffer()
			go func() {
				if _, err := io.Copy(&pipedOutputString, pipedOutput); err != nil {
					Fail(err.Error())
				}
			}()

			var pipedErrorString strings.Builder
			pipedError := cli.GetPipedErrorBuffer()
			go func() {
				if _, err := io.Copy(&pipedErrorString, pipedError); err != nil {
					Fail(err.Error())
				}
			}()

			err = cli.Run([]string{"aa", "bb"})
			Expect(err).NotTo(HaveOccurred())

			Expect(strings.Trim(outputBuffer.String(), "\r\n")).To(BeEquivalentTo("aa - written to stdout"))
			Expect(strings.Trim(pipedOutputString.String(), "\r\n")).To(BeEquivalentTo("aa - written to stdout"))
			Expect(strings.Trim(errorBuffer.String(), "\r\n")).To(BeEquivalentTo("bb - written to stderr"))
			Expect(strings.Trim(pipedErrorString.String(), "\r\n")).To(BeEquivalentTo("bb - written to stderr"))
		})

		It("runs cli and captures output written stdout and stderr and passed environment variable", func() {
			cli, err = run.NewCLI(echoOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.RunWithEnv([]string{"aa", "bb", "SOME_VAR"}, []string{"SOME_VAR=abcde"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Trim(outputBuffer.String(), "\r\n")).To(BeEquivalentTo("aa - SOME_VAR=abcde"))
			Expect(strings.Trim(errorBuffer.String(), "\r\n")).To(BeEquivalentTo("bb - SOME_VAR=abcde"))
		})

		It("runs cli and captures filtered output written to stdout and complete output to piped buffers", func() {

			var (
				filter streams.Filter
			)

			cli, err = run.NewCLI(echoOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			filter.AddExcludePattern("cccc")
			cli.ApplyFilter(&filter)

			var pipedOutputString strings.Builder
			pipedOutput := cli.GetPipedOutputBuffer()
			go func() {
				if _, err := io.Copy(&pipedOutputString, pipedOutput); err != nil {
					Fail(err.Error())
				}
			}()

			err = cli.Run([]string{"aaaa\nbbbb\ncccc\ndddd\n", ""})
			Expect(err).NotTo(HaveOccurred())

			Expect(outputBuffer.String()).To(BeEquivalentTo("aaaa\nbbbb\ndddd\n - written to stdout\n"))
			Expect(pipedOutputString.String()).To(BeEquivalentTo("aaaa\nbbbb\ncccc\ndddd\n - written to stdout\n"))
		})

		It("runs cli and captures filtered output written to stdout and to piped buffers", func() {

			var (
				filter streams.Filter
			)

			cli, err = run.NewCLI(echoOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			var pipedOutputString strings.Builder
			pipedOutput := cli.GetPipedOutputBuffer()
			go func() {
				if _, err := io.Copy(&pipedOutputString, pipedOutput); err != nil {
					Fail(err.Error())
				}
			}()

			filter.AddExcludePattern("cccc")
			cli.ApplyFilter(&filter)

			err = cli.Run([]string{"aaaa\nbbbb\ncccc\ndddd\n", ""})
			Expect(err).NotTo(HaveOccurred())

			Expect(outputBuffer.String()).To(BeEquivalentTo("aaaa\nbbbb\ndddd\n - written to stdout\n"))
			Expect(pipedOutputString.String()).To(BeEquivalentTo("aaaa\nbbbb\ndddd\n - written to stdout\n"))
		})
	})

	Context("run shell cli command async and test timeout", func() {

		var (
			timedOutput string
		)

		BeforeEach(func() {
			timedOutput, err = filepath.Abs(workingDirectory + "/../test/fixtures/cli/timed-output.sh")
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs CLI script for 5 seconds and validates it ended as expected", func() {
			cli, err = run.NewCLI(timedOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.Start([]string{"5"})
			Expect(err).NotTo(HaveOccurred())
			err = cli.Wait()
			Expect(outputBuffer.String()).To(Equal(outputRunFor5Secs))
		})

		It("runs CLI script and stops it after 2 seconds", func() {
			cli, err = run.NewCLI(timedOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.Start([]string{"60"})
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(3 * time.Second)
			err = cli.Stop()
			Expect(outputBuffer.String()).To(MatchRegexp(".*running for 2 secs\ninterrupted"))
		})
		
		It("runs CLI script with interrupts disabled for 3s and waits 2s for it to terminate", func() {
			cli, err = run.NewCLI(timedOutput, workingDirectory, &outputBuffer, &errorBuffer)
			Expect(err).NotTo(HaveOccurred())

			err = cli.Start([]string{"60", "X"})
			Expect(err).NotTo(HaveOccurred())
			err = cli.Wait(3 * time.Second, 2 * time.Second)
			Expect(outputBuffer.String()).To(MatchRegexp(".*running for 2 secs\ncannot interrupt\nrunning for 3 secs\nrunning for 4 secs"))
		})
	})
})

const outputRunFor5Secs = `running for 1 secs
running for 2 secs
running for 3 secs
running for 4 secs
running for 5 secs
`