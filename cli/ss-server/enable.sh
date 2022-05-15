#!/usr/bin/env bash

sudo systemctl enable ss
sudo systemctl start ss
sudo journalctl -u ss --output cat -f
