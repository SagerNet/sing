package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
)

func main() {
	var size int
	if len(os.Args) > 1 {
		size, _ = strconv.Atoi(os.Args[1])
	}
	if size == 0 {
		size = 32
	}
	psk := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, psk)
	if err != nil {
		logrus.Fatal(err)
	}
	os.Stdout.WriteString(base64.StdEncoding.EncodeToString(psk))
}
