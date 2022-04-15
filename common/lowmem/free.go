package lowmem

import (
	"runtime"
)

var Enabled = false

func Free() {
	if Enabled {
		runtime.GC()
	}
}
