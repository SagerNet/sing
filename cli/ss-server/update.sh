#!/usr/bin/env bash

git fetch &&
  git reset origin/main --hard &&
  git clean -fdx &&
  go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/ss-server &&
  sudo cp $(go env GOPATH)/bin/ss-server /usr/local/bin
