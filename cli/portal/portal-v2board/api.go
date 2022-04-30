package main

import (
	"context"
	"encoding/json"

	"github.com/go-resty/resty/v2"
	E "github.com/sagernet/sing/common/exceptions"
)

type NodeClient struct {
	client *resty.Client
}

func NewNodeClient(baseURL string, token string, node string) *NodeClient {
	r := resty.New()
	r.SetBaseURL(baseURL)
	r.SetQueryParams(map[string]string{
		"node_id": node,
		"token":   token,
	})
	// r.SetDebug(true)
	return &NodeClient{r}
}

type ShadowsocksUserList struct {
	Port   uint16
	Method string
	Users  map[int]string // id password
}

type RawShadowsocksUserList struct {
	Data []RawShadowsocksUser `json:"data"`
}

type RawShadowsocksUser struct {
	Id     int    `json:"id"`
	Port   uint16 `json:"port"`
	Cipher string `json:"cipher"`
	Secret string `json:"secret"`
}

func (c *NodeClient) GetShadowsocksUserList(ctx context.Context) (*ShadowsocksUserList, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(new(RawShadowsocksUserList)).
		Get("/api/v1/server/ShadowsocksTidalab/user")
	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, E.New("HTTP ", resp.StatusCode(), " ", resp.Body())
	}

	rawUserList := resp.Result().(*RawShadowsocksUserList)

	userList := &ShadowsocksUserList{
		Method: rawUserList.Data[0].Cipher,
		Port:   rawUserList.Data[0].Port,
		Users:  make(map[int]string),
	}

	for _, item := range rawUserList.Data {
		if item.Cipher != userList.Method {
			return nil, E.New("not unique method in item ", item.Id)
		}
		if item.Port != userList.Port {
			return nil, E.New("not unique port in item ", item.Id)
		}
		userList.Users[item.Id] = item.Secret
	}

	return userList, nil
}

type TrojanUserList struct {
	Users map[int]string // id password
}

type RawTrojanUserList struct {
	Msg  string          `json:"msg"`
	Data []RawTrojanUser `json:"data"`
}

type RawTrojanUser struct {
	ID             int   `json:"id"`
	T              int   `json:"t"`
	U              int64 `json:"u"`
	D              int64 `json:"d"`
	TransferEnable int64 `json:"transfer_enable"`
	TrojanUser     struct {
		Password string `json:"password"`
	} `json:"trojan_user"`
}

func (c *NodeClient) GetTrojanUserList(ctx context.Context) (*TrojanUserList, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(new(RawTrojanUserList)).
		Get("/api/v1/server/TrojanTidalab/user")
	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, E.New("HTTP ", resp.StatusCode(), " ", resp.String())
	}

	rawUserList := resp.Result().(*RawTrojanUserList)

	userList := &TrojanUserList{
		Users: make(map[int]string),
	}

	for _, item := range rawUserList.Data {
		userList.Users[item.ID] = item.TrojanUser.Password
	}

	return userList, nil
}

type TrojanConfig struct {
	LocalPort uint16
	SNI       string
}

type RawTrojanConfig struct {
	LocalPort uint16 `json:"local_port"`
	Ssl       struct {
		Sni string `json:"sni"`
	} `json:"ssl"`
}

func (c *NodeClient) GetTrojanConfig(ctx context.Context) (*TrojanConfig, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParam("local_port", "1").
		Get("/api/v1/server/TrojanTidalab/config")
	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, E.New("HTTP ", resp.StatusCode(), " ", resp.String())
	}

	rawConfig := new(RawTrojanConfig)
	err = json.Unmarshal(resp.Body(), rawConfig)
	if err != nil {
		return nil, E.Cause(err, "parse raw trojan config")
	}

	trojanConfig := new(TrojanConfig)
	trojanConfig.LocalPort = rawConfig.LocalPort
	trojanConfig.SNI = rawConfig.Ssl.Sni

	return trojanConfig, nil
}

type VMessUserList struct {
	AlterID int
	Users   map[int]string // id uuid
}

type RawVMessUserList struct {
	Msg  string         `json:"msg"`
	Data []RawVMessUser `json:"data"`
}

type RawVMessUser struct {
	Id             int   `json:"id"`
	T              int   `json:"t"`
	U              int64 `json:"u"`
	D              int64 `json:"d"`
	TransferEnable int64 `json:"transfer_enable"`
	V2RayUser      struct {
		Uuid    string `json:"uuid"`
		Email   string `json:"email"`
		AlterId int    `json:"alter_id"`
		Level   int    `json:"level"`
	} `json:"v2ray_user"`
}

func (c *NodeClient) GetVMessUserList(ctx context.Context) (*VMessUserList, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(new(RawVMessUserList)).
		Get("/api/v1/server/Deepbwork/user")
	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, E.New("HTTP ", resp.StatusCode(), " ", resp.String())
	}

	rawUserList := resp.Result().(*RawVMessUserList)

	userList := &VMessUserList{
		AlterID: rawUserList.Data[0].V2RayUser.AlterId,
		Users:   make(map[int]string),
	}

	for _, user := range rawUserList.Data {
		userList.Users[user.Id] = user.V2RayUser.Uuid
	}

	return userList, nil
}

type VMessConfig struct {
	Port     uint16
	Network  string
	Security string
}

type RawV2RayConfig struct {
	Inbounds []struct {
		Protocol       string `json:"protocol"`
		Port           uint16 `json:"port"`
		StreamSettings struct {
			Network  string `json:"network"`
			Security string `json:"security,omitempty"`
		} `json:"streamSettings,omitempty"`
	} `json:"inbounds"`
}

func (c *NodeClient) GetVMessConfig(ctx context.Context) (*VMessConfig, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParam("local_port", "1").
		Get("/api/v1/server/Deepbwork/config")
	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, E.New("HTTP ", resp.StatusCode(), " ", resp.String())
	}

	rawConfig := new(RawV2RayConfig)
	err = json.Unmarshal(resp.Body(), rawConfig)
	if err != nil {
		return nil, err
	}

	vmessConfig := new(VMessConfig)
	vmessConfig.Port = rawConfig.Inbounds[0].Port
	vmessConfig.Network = rawConfig.Inbounds[0].StreamSettings.Network
	vmessConfig.Security = rawConfig.Inbounds[0].StreamSettings.Security

	return vmessConfig, nil
}

type UserTraffic struct {
	UID      int   `json:"user_id"`
	Upload   int64 `json:"u"`
	Download int64 `json:"d"`
}

func (c *NodeClient) ReportShadowsocksTraffic(ctx context.Context, userTraffic []UserTraffic) error {
	return c.reportTraffic(ctx, "/api/v1/server/ShadowsocksTidalab/submit", userTraffic)
}

func (c *NodeClient) ReportVMessTraffic(ctx context.Context, userTraffic []UserTraffic) error {
	return c.reportTraffic(ctx, "/api/v1/server/Deepbwork/submit", userTraffic)
}

func (c *NodeClient) ReportTrojanTraffic(ctx context.Context, userTraffic []UserTraffic) error {
	return c.reportTraffic(ctx, "/api/v1/server/TrojanTidalab/submit", userTraffic)
}

func (c *NodeClient) reportTraffic(ctx context.Context, path string, userTraffic []UserTraffic) error {
	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(userTraffic).
		Post(path)
	if err != nil {
		return err
	}

	if !resp.IsSuccess() {
		return E.New("HTTP ", resp.StatusCode(), " ", resp.String())
	}
	return nil
}
