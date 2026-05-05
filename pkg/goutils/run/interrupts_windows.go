// +build windows

package run

import (
	"syscall"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

const (
	CTRL_C_EVENT        = uint32(0)
	CTRL_BREAK_EVENT    = uint32(1)
	CTRL_CLOSE_EVENT    = uint32(2)
	CTRL_LOGOFF_EVENT   = uint32(5)
	CTRL_SHUTDOWN_EVENT = uint32(6)
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

func __interruptEvent(handler func() bool) error {
	
	ok, _, err := procSetConsoleCtrlHandler.Call(
		syscall.NewCallback(func(controlType uint32) uint {
			if controlType == CTRL_C_EVENT ||
				controlType == CTRL_BREAK_EVENT ||
				controlType == CTRL_CLOSE_EVENT ||
				controlType == CTRL_LOGOFF_EVENT ||
				controlType == CTRL_SHUTDOWN_EVENT {

				logger.DebugMessage(
					"__interruptEvent: Received interrupt event: %d", 
					controlType,
				)
				if handler() {
					return 1
				}
			}
			return 0
		}),
		1,
	)
	if ok == 0 {
		return err
	}
	return nil
}

func init() {
	HandleInterruptEvent = __interruptEvent
}
