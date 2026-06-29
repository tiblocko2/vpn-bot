package config

import (
	"encoding/json"
	"os"
)

type InboundConfig struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

type Config struct {
	BotToken      string          `json:"bot_token"`
	SuperUserID   int64           `json:"super_user_id"`
	PanelURL      string          `json:"panel_url"`
	PanelUsername string          `json:"panel_username"`
	PanelPassword string          `json:"panel_password"`
	SubDomain     string          `json:"sub_domain"`
	ProxyURL      string          `json:"proxy_url"`
	Inbounds      []InboundConfig `json:"inbounds"`
	DBPath        string          `json:"db_path"`

	// XUIDBDSN is the PostgreSQL DSN of the 3X-UI database (v3.4.1+).
	// The bot reads clients/inbounds straight from it — it is the source of truth.
	// Example: postgres://user:pass@127.0.0.1:5432/xui?sslmode=disable
	XUIDBDSN string `json:"xui_db_dsn"`

	// PanelAPIToken enables Bearer-token auth (Settings → Security → API Tokens).
	// When set, cookie-based login is skipped entirely (no CSRF issues).
	PanelAPIToken string `json:"panel_api_token,omitempty"`
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

func AddInbound(ib InboundConfig) error {
	for _, existing := range Cfg.Inbounds {
		if existing.ID == ib.ID {
			return nil // already present
		}
	}
	Cfg.Inbounds = append(Cfg.Inbounds, ib)
	return Save()
}

func RemoveInbound(id int64) error {
	updated := Cfg.Inbounds[:0]
	for _, ib := range Cfg.Inbounds {
		if ib.ID != id {
			updated = append(updated, ib)
		}
	}
	Cfg.Inbounds = updated
	return Save()
}
