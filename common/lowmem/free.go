package lowmem

import (
	"runtime/debug"
)

var Enabled = false

func Free() {
	if Enabled {
		debug.FreeOSMemory()
	}
}
