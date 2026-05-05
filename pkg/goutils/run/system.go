package run

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mitchellh/go-homedir"
)

var (
	outputBuffer bytes.Buffer

	cliNameMapping map[string]string
	cliSearchPaths map[string][]string

	shell CLI
	sherr error
)

// creates a CLI by locating within the
// filesystem and system path.
func CreateCLI(cliName string, outputBuffer, errorBuffer io.Writer) (CLI, string, error) {

	var (
		err error

		cliPath string
	)

	cliBinaryName := getCliName(cliName)
	for _, path := range getCliSearchPaths(cliName) {
		if _, err = os.Stat(filepath.Join(path, cliBinaryName)); err == nil {
			cliPath = filepath.Join(path, cliBinaryName)
			break
		}
	}
	if len(cliPath) == 0 {
		if cliPath, err = LookupFilePathInSystem(cliBinaryName); err != nil {
			return nil, cliPath, err
		}
	}

	cwd, _ := os.Getwd()
	cli, err := NewCLI(cliPath, cwd, outputBuffer, errorBuffer)
	return cli, cliPath, err
}

// hook to transform a cli name to a
// system local specific name. i.e.
// a binary named foo in linux maybe
// name foo.exe in windows.
func AddCliNameMapping(cliName string, targetCliName string) {
	cliNameMapping[cliName] = targetCliName
}

func getCliName(cliName string) string {
	if targetCliName, ok := cliNameMapping[cliName]; ok {
		return targetCliName
	}
	return cliName
}

// hook to provide a list of paths
// to refer for the cli binary before
// attempting to locate it in the
// system path
func AddCliSearchPaths(cliName string, searchPaths... string) {
	cliSearchPaths[cliName] = append(cliSearchPaths[cliName], searchPaths...)
}

func getCliSearchPaths(cliName string) []string {
	if searchPaths, ok := cliSearchPaths[cliName]; ok {
		return searchPaths
	}
	return []string{}
}

// looks for the given file within
// the system path as set in the
// environment
func LookupFilePathInSystem(fileName string) (string, error) {

	var (
		err error
	)

	defer outputBuffer.Reset()

	if sherr == nil {
		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "openbsd" {
			if err = shell.Run([]string{"-c", fmt.Sprintf("which %s", fileName)}); err != nil {
				return "", fmt.Errorf(
					"error looking up file '%s' in system path: %s",
					fileName, strings.TrimSuffix(outputBuffer.String(), "\n"),
				)
			}
			return strings.TrimSuffix(outputBuffer.String(), "\n"), nil

		} else if runtime.GOOS == "windows" {
			if err = shell.Run([]string{"/C", fmt.Sprintf("where %s", fileName)}); err != nil {
				return "", fmt.Errorf(
					"error looking up file '%s' in system path: %s",
					fileName, strings.TrimSuffix(outputBuffer.String(), "\r\n"),
				)
			}
			return strings.TrimSuffix(outputBuffer.String(), "\r\n"), nil
		}
	}
	return "", sherr
}

func init() {
	cliNameMapping = make(map[string]string)
	cliSearchPaths = make(map[string][]string)

	home, _ := homedir.Dir()
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "openbsd" {
		shell, sherr = NewCLI("/bin/sh", home, &outputBuffer, &outputBuffer)
	} else if runtime.GOOS == "windows" {
		shell, sherr = NewCLI("C:\\Windows\\System32\\cmd.exe", home, &outputBuffer, &outputBuffer)
	} else {
		sherr = fmt.Errorf("unsupported OS")
	}
}
