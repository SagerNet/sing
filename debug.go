//go:build debug

package sing

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe(":8964", nil)
}
