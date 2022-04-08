package main

import (
	"encoding/binary"
	"github.com/klauspost/compress/zstd"
	"github.com/sagernet/sing/common/rw"
	"io"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/ulikunitz/xz"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

func main() {
	response, err := http.Get("https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat.xz")
	if err != nil {
		logrus.Fatal(err)
	}
	defer response.Body.Close()
	reader, err := xz.NewReader(response.Body)
	if err != nil {
		logrus.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		logrus.Fatal(err)
	}
	geosite := routercommon.GeoSiteList{}
	if err = proto.Unmarshal(data, &geosite); err != nil {
		logrus.Fatal(err)
	}
	output, err := os.Create("geosite.dat")
	if err != nil {
		logrus.Fatal(err)
	}
	binary.Write(output, binary.BigEndian, byte(0)) // version
	writer, _ := zstd.NewWriter(output)
	rw.WriteUVariant(writer, uint64(len(geosite.Entry)))
	for _, site := range geosite.Entry {
		rw.WriteVString(writer, site.CountryCode)
		domains := make([]string, 0, len(site.Domain))
		for _, domain := range site.Domain {
			if domain.Type == routercommon.Domain_Full {
				domains = append(domains, domain.Value)
			} else {
				domains = append(domains, domain.Value)
				domains = append(domains, "."+domain.Value)
			}
		}
		rw.WriteUVariant(writer, uint64(len(domains)))
		for _, domain := range domains {
			rw.WriteVString(writer, domain)
		}
	}
	writer.Close()
	output.Close()
}
