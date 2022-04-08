//go:build debug

package main

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe("127.0.0.1:8964", nil)
}
