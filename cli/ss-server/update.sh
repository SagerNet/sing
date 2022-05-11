#!/usr/bin/env bash

export GOAMD64=v3

DIR=$(dirname "$0")
PROJECT=$DIR/../..

pushd $PROJECT
git fetch || exit 1
git reset origin/main --hard || exit 1
git clean -fdx || exit 1
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/ss-server || exit 1
popd

sudo systemctl stop ss
sudo cp $(go env GOPATH)/bin/ss-server /usr/local/bin || exit 1
sudo systemctl start ss
sudo journalctl -u ss --output cat -f
