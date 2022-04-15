# sing

Do you hear the people sing?

### geosite

```shell
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/geosite
```

create from v2ray

`geosite add v2ray`

create cn only dat

`geosite add v2ray cn`

### geoip

```shell
wget 'https://github.com/Dreamacro/maxmind-geoip/releases/latest/download/Country.mmdb'
```

### ss-local

```shell
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/ss-local
```