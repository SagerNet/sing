//go:build !disable_unsafe

package buf

import (
	"os"
	_ "unsafe"
)

//go:linkname parsedebugvars runtime.parsedebugvars
func parsedebugvars()

func init() {
	disableInvalidPtrCheck()
}

func disableInvalidPtrCheck() {
	debug := os.Getenv("GODEBUG")
	if debug == "" {
		os.Setenv("GODEBUG", "invalidptr=0")
	} else {
		os.Setenv("GODEBUG", debug+",invalidptr=0")
	}
	parsedebugvars()
}
