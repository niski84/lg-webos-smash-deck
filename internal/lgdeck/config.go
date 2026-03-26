package lgdeck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AppConfig holds persisted settings for the LG WebOS Smash Deck server.
type AppConfig struct {
	Port      string `json:"port"`
	TVIP      string `json:"tv_ip"`
	TVMac     string `json:"tv_mac"`
	Keycode   string `json:"keycode"`
	MaxVolume int    `json:"max_volume"` // 0 means "use default 100"
}

// DataDir returns the directory used for all persistent data files.
// Reads DATA_DIR from env; defaults to "./data".
func DataDir() string {
	if d := strings.TrimSpace(os.Getenv("DATA_DIR")); d != "" {
		return d
	}
	return "data"
}

func DefaultSettingsPath() string {
	return filepath.Join(DataDir(), "lgdeck-settings.json")
}

func LoadAppConfig(path string) AppConfig {
	cfg := AppConfig{
		Port:    getenv("PORT", "8088"),
		TVIP:    getenv("TV_IP", ""),
		TVMac:   getenv("TV_MAC", ""),
		Keycode: getenv("TV_KEYCODE", ""),
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		var stored AppConfig
		if json.Unmarshal(raw, &stored) == nil {
			if stored.Port != "" {
				cfg.Port = stored.Port
			}
			if stored.TVIP != "" {
				cfg.TVIP = stored.TVIP
			}
			if stored.TVMac != "" {
				cfg.TVMac = stored.TVMac
			}
			if stored.Keycode != "" {
				cfg.Keycode = stored.Keycode
			}
			if stored.MaxVolume > 0 {
				cfg.MaxVolume = stored.MaxVolume
			}
		}
	}
	// Env vars override file when explicitly set.
	if v := strings.TrimSpace(os.Getenv("TV_IP")); v != "" {
		cfg.TVIP = v
	}
	if v := strings.TrimSpace(os.Getenv("TV_MAC")); v != "" {
		cfg.TVMac = v
	}
	if v := strings.TrimSpace(os.Getenv("TV_KEYCODE")); v != "" {
		cfg.Keycode = v
	}
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		cfg.Port = p
	}
	return cfg
}

func SaveAppConfig(path string, cfg AppConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func maskSecret(s string) string {
	if len(s) < 4 {
		if s == "" {
			return ""
		}
		return "****"
	}
	return s[:2] + strings.Repeat("•", len(s)-2)
}

func isMasked(s string) bool {
	return strings.Contains(s, "•") || strings.HasPrefix(s, "****")
}
