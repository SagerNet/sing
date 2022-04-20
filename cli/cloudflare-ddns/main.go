package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/netip"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sagernet/sing"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vishvananda/netlink"
)

var configPath string

func main() {
	command := &cobra.Command{
		Use:     "cloudflare-ddns [-c config.json]",
		Run:     run,
		Version: sing.VersionStr,
	}
	command.Flags().StringVarP(&configPath, "config", "c", "config.json", "set config path")
	if err := command.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

type Config struct {
	APIKey    string `json:"api_key"`
	APIEmail  string `json:"api_email"`
	Domain    string `json:"domain"`
	OverProxy bool   `json:"over_proxy"`
}

var (
	domain    string
	overProxy bool
)

var (
	flare *cloudflare.API
	zone  *cloudflare.Zone
)

func run(cmd *cobra.Command, args []string) {
	c := new(Config)
	cc, err := ioutil.ReadFile(configPath)
	if err != nil {
		logrus.Fatal(err)
	}
	err = json.Unmarshal(cc, c)
	if err != nil {
		logrus.Fatal(err)
	}
	domain = c.Domain
	overProxy = c.OverProxy

	flare, err = cloudflare.New(c.APIKey, c.APIEmail)
	if err != nil {
		logrus.Fatal(err)
	}

	zone, err = findZoneForDomain(domain)
	if err != nil {
		logrus.Fatal(err)
	}

	checkUpdate()

	events := make(chan netlink.AddrUpdate, 1)
	err = netlink.AddrSubscribe(events, nil)
	if err != nil {
		logrus.Fatal(err)
	}

	for event := range events {
		addr, _ := netip.AddrFromSlice(event.LinkAddress.IP)
		if !N.IsPublicAddr(addr) {
			continue
		}
		checkUpdate()
	}
}

func findZoneForDomain(domain string) (*cloudflare.Zone, error) {
	zones, err := flare.ListZones(context.Background())
	if err != nil {
		return nil, err
	}
	for _, z := range zones {
		if strings.HasSuffix(domain, z.Name) {
			return &z, nil
		}
	}
	return nil, E.New("unable to find zone for domain ", domain)
}

func checkUpdate() {
	addrs, err := N.LocalPublicAddrs()
	if err != nil {
		logrus.Fatal(err)
	}
	addrMap := make(map[string]netip.Addr)
	for _, addr := range addrs {
		addrMap[addr.String()] = addr
	}

	if common.IsEmpty(addrs) {
		logrus.Warn("this device has no public addresses!")
	}

	records, err := flare.DNSRecords(context.Background(), zone.ID, cloudflare.DNSRecord{
		Name: domain,
	})
	if err != nil {
		logrus.Fatal(err)
	}

	for _, record := range records {
		if !(record.Type == "A" || record.Type == "AAAA") {
			continue
		}
		if _, exists := addrMap[record.Content]; !exists || record.Proxied == nil && overProxy || record.Proxied != nil && *record.Proxied != overProxy {
			logrus.Info("Deleting ", record.Type, " ", record.Content)
			err = flare.DeleteDNSRecord(context.Background(), zone.ID, record.ID)
			if err != nil {
				logrus.Fatal(err)
			}
		} else {
			delete(addrMap, record.Content)
		}
	}
	for content, addr := range addrMap {
		record := cloudflare.DNSRecord{
			Name:    domain,
			Content: content,
			Proxied: &overProxy,
		}
		if addr.Is4() {
			record.Type = "A"
		} else {
			record.Type = "AAAA"
		}
		logrus.Info("Adding ", record)
		_, err = flare.CreateDNSRecord(context.Background(), zone.ID, record)
		if err != nil {
			logrus.Fatal(err)
		}
	}
}
