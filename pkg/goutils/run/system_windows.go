// +build windows

package run

import (
	"fmt"
	"syscall"
)

var drives []string

// returns the logical drives in 
// the host windows environment
func GetLogicalDrives() ([]string, error) {

	var (
		err syscall.Errno

		ret uintptr
	)

	if len(drives) == 0 {
		availableDrives := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"}

		kernel32, _ := syscall.LoadLibrary("kernel32.dll")
		getLogicalDrivesHandle, _ := syscall.GetProcAddress(kernel32, "GetLogicalDrives")
	
		if ret, _, err = syscall.SyscallN(uintptr(getLogicalDrivesHandle), 0, 0, 0, 0); err != 0 {
			return nil, fmt.Errorf("Unable to enumerate the systems logical drives: %d", err)
		}
		bitMap := uint32(ret)
		for i := range availableDrives {
			if bitMap&1 == 1 {
				drives = append(drives, availableDrives[i]+":\\")
			}
			bitMap >>= 1
		}
	}
	return drives, nil
}
