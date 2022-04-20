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

### ddns

```shell
GOBIN=/usr/local/bin/ go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/cloudflare-ddns

cat > /usr/local/etc/ddns.json <<EOF
{
  "cloudflare_api_key": "",
  "cloudflare_api_email": "",
  "domain": "example.com",
  "over_proxy": false
}
EOF

sudo cp ./cli/cloudflare-ddns/ddns.service /etc/systemd/system
sudo systemctl daemon-reload
sudo systemctl enable ddns
sudo systemctl start ddns
```