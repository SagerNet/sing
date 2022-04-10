# sing

Do you hear the people sing?

```shell
# geo resources
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/get-geoip
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/gen-geosite

# ss-local
go install -v -trimpath -ldflags "-s -w -buildid=" ./cli/ss-local
```