// +build linux darwin

package term

const (

	// Terminal font colors
	WHITE      = "\033[1;37m"
	LIGHT_GRAY = "\033[0;37m"

	BLACK     = "\033[0;30m"
	DARK_GRAY = "\033[1;30m"

	RED    = "\033[31m"
	GREEN  = "\033[32m"
	YELLOW = "\033[33m"
	BLUE   = "\033[34m"
	PURPLE = "\033[35m"
	CYAN   = "\033[36m"

	// Terminal font attributes
	BOLD      = "\033[1m"
	ITALIC    = "\033[3m"
	UNDERLINE = "\033[4m"
	HIGHLIGHT = "\033[7m"
	NORMAL    = "\033[22m"
	DIM       = "\033[2m"

	// Reset formatting
	NC = "\033[0m"
)
