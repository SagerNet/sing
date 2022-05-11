# ss-server

## Requirements

```
* Go 1.18
```

## Install

```shell
git clone https://github.com/SagerNet/sing
cd sing

cli/ss-server/install.sh

sudo systemctl enable ss
sudo systemctl start ss
```

## Log

```shell
journalctl -u ss --output cat -f
```

## Update

```shell
sudo systemctl stop ss
cli/ss-server/update.sh
sudo systemctl start ss
```

## Uninstall

```shell
sudo systemctl stop ss
cli/ss-server/uninstall.sh
```