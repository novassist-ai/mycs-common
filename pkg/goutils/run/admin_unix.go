//go:build linux || darwin
// +build linux darwin

package run

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

func IsAdmin() (bool, error) {
	return os.Geteuid() == 0, nil
}

func RunAsAdmin(outputBuffer, errorBuffer io.Writer) error {
	return RunAsAdminWithArgs(os.Args, outputBuffer, errorBuffer)
}

func RunAsAdminWithArgs(cmdArgs []string, outputBuffer, errorBuffer io.Writer) error {

	var (
		err error
		cli CLI

		workingDirectory string
	)

	if workingDirectory, err = os.Getwd(); err != nil {
		return nil
	}
	if cli, err = NewCLI(
		"/usr/bin/sudo", 
		workingDirectory,
		outputBuffer,
		errorBuffer,
	); err != nil {
		return err
	}
	args := []string{ "-s", "-E" }
	args = append(args, cmdArgs...)
	return cli.RunWithEnv(args, []string{"__CB_RUN_AS_ROOT__=1"})
}

func TerminateProcess(psRE string, outputBuffer, errorBuffer io.Writer) error {

	var (
		err error

		ps       CLI
		psOutput strings.Builder
	)

	if ps, err = NewCLI("/bin/ps", os.TempDir(), &psOutput, &psOutput); err != nil {
		return err
	}
	if err = ps.Run([]string{"-ef"}); err != nil {
		return err
	}
	pattern := regexp.MustCompile(psRE)
	scanner := bufio.NewScanner(strings.NewReader(psOutput.String()))
	for scanner.Scan() {
		line := scanner.Text()
		matches := pattern.FindAllSubmatch([]byte(line), -1)
		if len(matches) > 0 && len(matches[0]) == 3 {
			pid := matches[0][2]
			if err = RunAsAdminWithArgs([]string{ "kill", "-15", string(pid) }, outputBuffer, errorBuffer); err != nil {
				return err
			} else {
				return nil
			}
		}
	}
	return fmt.Errorf("not found")
}