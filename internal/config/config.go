package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	BotToken       string `json:"bot_token"`
	SuperUserID    int64  `json:"super_user_id"`
	PanelURL       string `json:"panel_url"`
	PanelUsername  string `json:"panel_username"`
	PanelPassword  string `json:"panel_password"`
	SubDomain      string `json:"sub_domain"`
	ProxyURL       string `json:"proxy_url"`
	VlessInboundID int64  `json:"vless_inbound_id"`
	VmessInboundID int64  `json:"vmess_inbound_id"`
	DBPath         string `json:"db_path"`
}

var Cfg *Config

var configPath string

func Load(path string) error {
	configPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c := &Config{}
	if err := json.Unmarshal(data, c); err != nil {
		return err
	}
	Cfg = c
	return nil
}

func Save() error {
	data, err := json.MarshalIndent(Cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0600)
}

func SetSubDomain(domain string) error {
	Cfg.SubDomain = domain
	return Save()
}
