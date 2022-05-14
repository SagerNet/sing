package buf

import (
	"os"
	"runtime"
	_ "unsafe"
)

//go:linkname parsedebugvars runtime.parsedebugvars
func parsedebugvars()

//noinspection GoBoolExpressions
func init() {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		disableInvalidPtrCheck()
	}
}

func disableInvalidPtrCheck() {
	debug := os.Getenv("GODEBUG")
	if debug == "" {
		os.Setenv("GODEBUG", "invalidptr=0")
	} else {
		os.Setenv("GODEBUG", "invalidptr=0,"+debug)
	}
	parsedebugvars()
}
