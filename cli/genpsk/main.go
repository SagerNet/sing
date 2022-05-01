package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

func main() {
	var psk [32]byte
	_, err := io.ReadFull(rand.Reader, psk[:])
	if err != nil {
		logrus.Fatal(err)
	}
	os.Stdout.WriteString(base64.StdEncoding.EncodeToString(psk[:]))
}
