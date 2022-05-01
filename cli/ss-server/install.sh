#!/usr/bin/env bash

go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/ss-server &&
  sudo cp $(go env GOPATH)/bin/ss-server /usr/local/bin/ &&
  sudo mkdir -p /usr/local/etc/shadowsocks &&
  sudo cp ./cli/ss-server/config.json /usr/local/etc/shadowsocks/config.json &&
  sudo sed -i "s|psk|$(go run ./cli/genpsk)|" /usr/local/etc/shadowsocks/config.json &&
  sudo cat /usr/local/etc/shadowsocks/config.json &&
  sudo cp ./cli/ss-server/ss.service /etc/systemd/system &&
  sudo cp ./cli/ss-server/ss@.service /etc/systemd/system &&
  sudo systemctl daemon-reload
