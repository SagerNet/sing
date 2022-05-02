package debug

import (
	"runtime/debug"
)

func Free() {
	if Enabled {
		debug.FreeOSMemory()
	}
}
