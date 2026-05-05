// +build windows

package run

import (
	"io"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func RunAsAdmin(outputBuffer, errorBuffer io.Writer) error {
	return RunAsAdminWithArgs(os.Args, outputBuffer, errorBuffer)
}

func RunAsAdminWithArgs(cmdArgs []string, outputBuffer, errorBuffer io.Writer) error {

	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	var showCmd int32 = 1 //SW_NORMAL

	return windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
}

func TerminateProcess(psRE string) error {
	return nil
}

func IsAdmin() (bool, error) {
	
	var (
		err error

		sid   *windows.SID
		admin bool
	)

	// Although this looks scary, it is directly copied from the
	// official windows documentation. The Go API for this is a
	// direct wrap around the official C++ API.
	//
	// See https://docs.microsoft.com/en-us/windows/desktop/api/securitybaseapi/nf-securitybaseapi-checktokenmembership
	if err = windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	); err != nil {
		return false, err
	}

	// This appears to cast a null pointer. See:
	//
	// https://github.com/golang/go/issues/28804#issuecomment-438838144
	token := windows.Token(0)
	
	if admin, err = token.IsMember(sid); err != nil {
		return false, err
	}

	// Also note that an admin is _not_ necessarily considered elevated.
	// For elevation see https://github.com/mozey/run-as-admin
	return admin /*&& token.IsElevated()*/, nil
}
