#!/bin/bash

build() {
    go build -v -o "$1" -trimpath -buildinfo=false -buildvcs=false -ldflags "-s -w -buildid=" ./example/shadowboom
}

export GOARCH=amd64
build sing_shadowboom_amd64

export GOARCH=386
build sing_shadowboom_386

export GOARCH=arm64
build sing_shadowboom_arm64

export GOOS=windows

export GOARCH=amd64
build sing_shadowboom_amd64.exe

export GOARCH=386
build sing_shadowboom_386.exe

export GOARCH=arm64
build sing_shadowboom_arm64.exe