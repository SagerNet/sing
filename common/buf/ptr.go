//go:build !disable_unsafe

package buf

import (
	"unsafe"
)

type dbgVar struct {
	name  string
	value *int32
}

//go:linkname dbgvars runtime.dbgvars
var dbgvars any

// go.info.runtime.dbgvars: relocation target go.info.[]github.com/sagernet/sing/common/buf.dbgVar not defined
//var dbgvars []dbgVar

func init() {
	debugVars := *(*[]dbgVar)(unsafe.Pointer(&dbgvars))
	for _, v := range debugVars {
		if v.name == "invalidptr" {
			*v.value = 0
			return
		}
	}
	panic("can't disable invalidptr")
}
