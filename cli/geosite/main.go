package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"strings"

	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/geosite"
	_ "github.com/sagernet/sing/common/log"
	N "github.com/sagernet/sing/common/network"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/ulikunitz/xz"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

var path string

func main() {
	command := &cobra.Command{
		Use: "geosite ...",
	}
	command.PersistentFlags().StringVarP(&path, "file", "f", "geosite.dat", "set resource path")
	command.AddCommand(&cobra.Command{
		Use:    "list",
		Short:  "List codes",
		PreRun: load,
		Run:    edit,
	})
	command.AddCommand(&cobra.Command{
		Use:    "keep",
		Short:  "Keep selected codes",
		PreRun: load,
		Run:    keep,
		Args:   cobra.MinimumNArgs(1),
	})
	command.AddCommand(&cobra.Command{
		Use:    "add <v2ray | loyalsoldier | path | url> [code]...",
		Short:  "Add codes form external file or url",
		PreRun: load0,
		Run:    add,
		Args:   cobra.MinimumNArgs(1),
	})
	if err := command.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

var site map[string][]string

func load(cmd *cobra.Command, args []string) {
	geoFile, err := os.Open(path)
	if err != nil {
		logrus.Fatal(E.Cause(err, "open geo resources"))
	}
	defer geoFile.Close()
	site, err = geosite.Read(geoFile)
	if err != nil {
		logrus.Fatal(E.Cause(err, "read geo resources"))
	}
}

func load0(cmd *cobra.Command, args []string) {
	geoFile, err := os.Open(path)
	if err == nil {
		defer geoFile.Close()
		site, err = geosite.Read(geoFile)
		if err != nil {
			logrus.Fatal(E.Cause(err, "read geo resources"))
		}
	}
	site = make(map[string][]string)
}

func edit(cmd *cobra.Command, args []string) {
	for code := range site {
		println(strings.ToLower(code))
	}
}

func keep(cmd *cobra.Command, args []string) {
	kept := make(map[string][]string)
	for _, code := range args {
		code = strings.ToUpper(code)
		if domains, exists := site[code]; exists {
			kept[code] = domains
		} else {
			logrus.Fatal("code ", strings.ToLower(code), " do not exists!")
		}
	}
	geoFile, err := os.Create(path)
	if err != nil {
		logrus.Fatal(err)
	}
	defer geoFile.Close()
	err = geosite.Write(geoFile, kept)
	if err != nil {
		logrus.Fatal(err)
	}
}

func add(cmd *cobra.Command, args []string) {
	resource := args[0]
find:
	switch resource {
	case "v2ray":
		for _, dir := range []string{
			"/usr/share/v2ray",
			"/usr/local/share/v2ray",
			"/opt/share/v2ray",
		} {
			file := dir + "/geosite.dat"
			if common.FileExists(file) {
				resource = file
				break find
			}
		}

		resource = "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat.xz"
	case "loyalsoldier":
		resource = "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat"
	}

	var data []byte
	var err error
	if strings.HasPrefix(resource, "http://") || strings.HasPrefix(resource, "https://") {
		logrus.Info("download ", resource)
		data, err = N.Get(resource)
		if err != nil {
			logrus.Fatal(err)
		}
	} else {
		logrus.Info("open ", resource)
		file, err := os.Open(resource)
		if err != nil {
			logrus.Fatal(err)
		}
		data, err = ioutil.ReadAll(file)
		file.Close()
		if err != nil {
			logrus.Fatal("read ", resource, ": ", err)
		}
	}
	{
		if strings.HasSuffix(resource, ".xz") {
			decoder, err := xz.NewReader(bytes.NewReader(data))
			if err == nil {
				data, _ = ioutil.ReadAll(decoder)
			}
		}
	}
	loaded := make(map[string][]string)
	{
		geositeList := routercommon.GeoSiteList{}
		err = proto.Unmarshal(data, &geositeList)
		if err == nil {
			for _, geoSite := range geositeList.Entry {
				domains := make([]string, 0, len(geoSite.Domain))
				for _, domain := range geoSite.Domain {
					if domain.Type == routercommon.Domain_Full {
						domains = append(domains, domain.Value)
					} else if domain.Type == routercommon.Domain_RootDomain {
						domains = append(domains, "+."+domain.Value)
					} else if domain.Type == routercommon.Domain_Plain {
						logrus.Warn("ignore match rule ", geoSite.CountryCode, " ", domain.Value)
					} else {
						domains = append(domains, "regexp:"+domain.Value)
					}
				}
				loaded[strings.ToLower(geoSite.CountryCode)] = common.Uniq(domains)
			}
			goto finish
		}
	}
	{
		loaded, _ = geosite.Read(bytes.NewReader(data))
	}
finish:
	if len(loaded) == 0 {
		logrus.Fatal("unknown resource format")
	}
	if len(args) > 1 {
		for _, code := range args[1:] {
			code = strings.ToLower(code)
			if domains, exists := loaded[code]; exists {
				site[code] = domains
			} else {
				logrus.Fatal("code ", code, " do not exists!")
			}
		}
	} else {
		for code, domains := range loaded {
			site[code] = domains
		}
	}
	for code, domains := range site {
		site[code] = common.Uniq(domains)
	}
	geoFile, err := os.Create(path)
	if err != nil {
		logrus.Fatal(err)
	}
	defer geoFile.Close()
	err = geosite.Write(geoFile, site)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Info("saved ", path)
}
