#!/usr/bin/env bash

sudo systemctl stop ss
sudo systemctl stop 'ss@*'
sudo rm -rf /usr/local/bin/ss-server
sudo rm -rf /usr/local/etc/shadowsocks
sudo rm -rf /etc/systemd/system/ss.service
sudo systemctl daemon-reload
