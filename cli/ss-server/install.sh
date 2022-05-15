#!/usr/bin/env bash

export GOAMD64=v3

go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/ss-server || exit 1
sudo cp $(go env GOPATH)/bin/ss-server /usr/local/bin/ || exit 1
sudo mkdir -p /usr/local/etc/shadowsocks || exit 1
sudo cp ./cli/ss-server/config.json /usr/local/etc/shadowsocks/config.json || exit 1
echo ">> /usr/local/etc/shadowsocks/config.json"
sudo sed -i "s|psk|$(go run ./cli/genpsk 16)|" /usr/local/etc/shadowsocks/config.json || exit 1
sudo cat /usr/local/etc/shadowsocks/config.json || exit 1
sudo cp ./cli/ss-server/ss.service /etc/systemd/system || exit 1
sudo systemctl daemon-reload
