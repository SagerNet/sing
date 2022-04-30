package cloudflare

import (
	"context"
	"strings"
	"sync"

	"github.com/cloudflare/cloudflare-go"
	E "github.com/sagernet/sing/common/exceptions"
)

type metaClient struct {
	clientEdit *cloudflare.API // needs Zone/DNS/Edit permissions
	clientRead *cloudflare.API // needs Zone/Zone/Read permissions

	zones   map[string]string // caches calls to ZoneIDByName
	zonesMu *sync.RWMutex
}

func newClient(config *Config) (*metaClient, error) {
	// with AuthKey/AuthEmail we can access all available APIs
	if config.AuthToken == "" {
		client, err := cloudflare.New(config.AuthKey, config.AuthEmail, cloudflare.HTTPClient(config.HTTPClient))
		if err != nil {
			return nil, err
		}

		return &metaClient{
			clientEdit: client,
			clientRead: client,
			zones:      make(map[string]string),
			zonesMu:    &sync.RWMutex{},
		}, nil
	}

	dns, err := cloudflare.NewWithAPIToken(config.AuthToken, cloudflare.HTTPClient(config.HTTPClient))
	if err != nil {
		return nil, err
	}

	if config.ZoneToken == "" || config.ZoneToken == config.AuthToken {
		return &metaClient{
			clientEdit: dns,
			clientRead: dns,
			zones:      make(map[string]string),
			zonesMu:    &sync.RWMutex{},
		}, nil
	}

	zone, err := cloudflare.NewWithAPIToken(config.ZoneToken, cloudflare.HTTPClient(config.HTTPClient))
	if err != nil {
		return nil, err
	}

	return &metaClient{
		clientEdit: dns,
		clientRead: zone,
		zones:      make(map[string]string),
		zonesMu:    &sync.RWMutex{},
	}, nil
}

func (m *metaClient) CreateDNSRecord(ctx context.Context, zoneID string, rr cloudflare.DNSRecord) (*cloudflare.DNSRecordResponse, error) {
	return m.clientEdit.CreateDNSRecord(ctx, zoneID, rr)
}

func (m *metaClient) DNSRecords(ctx context.Context, zoneID string, rr cloudflare.DNSRecord) ([]cloudflare.DNSRecord, error) {
	return m.clientEdit.DNSRecords(ctx, zoneID, rr)
}

func (m *metaClient) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	return m.clientEdit.DeleteDNSRecord(ctx, zoneID, recordID)
}

func (m *metaClient) ZoneIDByName(fqdn string) (string, error) {
	if fqdn[len(fqdn)-1] == '.' {
		fqdn = fqdn[:len(fqdn)-1]
	}

	m.zonesMu.RLock()

	for name, zoneId := range m.zones {
		if strings.HasSuffix(fqdn, name) {
			return zoneId, nil
		}
	}

	m.zonesMu.RUnlock()

	zones, err := m.clientRead.ListZones(context.Background())
	if err != nil {
		return "", err
	}

	m.zonesMu.Lock()
	for _, zone := range zones {
		m.zones[zone.Name] = zone.ID
	}
	m.zonesMu.Unlock()

	for name, zoneId := range m.zones {
		if strings.HasSuffix(fqdn, name) {
			return zoneId, nil
		}
	}

	return "", E.New("zone not found for domain ", fqdn)
}
