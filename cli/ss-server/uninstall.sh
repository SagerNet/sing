#!/usr/bin/env bash

sudo systemctl stop ss
sudo systemctl stop 'ss@*'
sudo rm /usr/local/bin/ss-server
sudo rm -r /usr/local/etc/shadowsocks
sudo rm /etc/systemd/system/ss.service
sudo systemctl daemon-reload
