#!/usr/bin/env bash

export GOAMD64=v3

DIR=$(dirname "$0")
PROJECT=$DIR/../..
PATH="$PATH:$(go env GOPATH)/bin"

pushd $PROJECT
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/genpsk || exit 1
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/ss-server || exit 1
popd

sudo cp $(go env GOPATH)/bin/ss-server /usr/local/bin/ || exit 1
sudo mkdir -p /usr/local/etc/shadowsocks || exit 1
sudo cp $DIR/config.json /usr/local/etc/shadowsocks/config.json || exit 1
echo ">> /usr/local/etc/shadowsocks/config.json"
sudo sed -i "s|psk|$(genpsk 16)|" /usr/local/etc/shadowsocks/config.json || exit 1
sudo cat /usr/local/etc/shadowsocks/config.json || exit 1
sudo cp $DIR/ss.service /etc/systemd/system || exit 1
sudo systemctl daemon-reload
