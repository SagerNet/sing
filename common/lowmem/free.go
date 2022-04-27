package lowmem

import (
	"runtime/debug"
)

func Free() {
	if Enabled {
		debug.FreeOSMemory()
	}
}
