package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSoftQuota = int64(5 * 1024 * 1024 * 1024)
	defaultReserve   = int64(1 * 1024 * 1024 * 1024)
)

type Config struct {
	ListenAddr string
	DataDir    string
	ArchiveDir string
	ThorDir    string
	WindowsDir string

	SyncthingURL             string
	SyncthingAPIKey          string
	SyncthingThorFolderID    string
	SyncthingWindowsFolderID string
	ReconcileInterval        time.Duration

	AuthMode           string
	CloudflareIssuer   string
	CloudflareAudience string
	CloudflareJWKSURL  string
	AdminEmail         string
	AllowedOrigin      string

	SoftQuotaBytes int64
	ReserveBytes   int64
}

func Load() (Config, error) {
	dataDir := env("THORSYNC_DATA_DIR", "./data")
	cfg := Config{
		ListenAddr:               env("THORSYNC_LISTEN_ADDR", ":8080"),
		DataDir:                  filepath.Clean(dataDir),
		ArchiveDir:               filepath.Clean(env("THORSYNC_ARCHIVE_DIR", "./archive")),
		ThorDir:                  filepath.Clean(env("THORSYNC_THOR_DIR", "./sync/thor")),
		WindowsDir:               filepath.Clean(env("THORSYNC_WINDOWS_DIR", "./sync/windows")),
		SyncthingURL:             strings.TrimRight(env("THORSYNC_SYNCTHING_URL", "http://syncthing:8384"), "/"),
		SyncthingThorFolderID:    env("THORSYNC_SYNCTHING_THOR_FOLDER_ID", "thorsync-thor"),
		SyncthingWindowsFolderID: env("THORSYNC_SYNCTHING_WINDOWS_FOLDER_ID", "thorsync-windows"),
		AuthMode:                 strings.ToLower(env("THORSYNC_AUTH_MODE", "cloudflare")),
		CloudflareIssuer:         strings.TrimRight(os.Getenv("THORSYNC_CF_ISSUER"), "/"),
		CloudflareJWKSURL:        os.Getenv("THORSYNC_CF_JWKS_URL"),
		AllowedOrigin:            os.Getenv("THORSYNC_ALLOWED_ORIGIN"),
		SoftQuotaBytes:           envInt64("THORSYNC_SOFT_QUOTA_BYTES", defaultSoftQuota),
		ReserveBytes:             envInt64("THORSYNC_RESERVE_BYTES", defaultReserve),
		ReconcileInterval:        envDuration("THORSYNC_RECONCILE_INTERVAL", 15*time.Minute),
	}

	var err error
	if cfg.SyncthingAPIKey, err = secret("THORSYNC_SYNCTHING_API_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.CloudflareAudience, err = secret("THORSYNC_CF_AUDIENCE"); err != nil {
		return Config{}, err
	}
	if cfg.AdminEmail, err = secret("THORSYNC_ADMIN_EMAIL"); err != nil {
		return Config{}, err
	}
	cfg.AdminEmail = strings.ToLower(strings.TrimSpace(cfg.AdminEmail))

	if cfg.AuthMode != "cloudflare" && cfg.AuthMode != "disabled" {
		return Config{}, fmt.Errorf("THORSYNC_AUTH_MODE must be cloudflare or disabled")
	}
	if cfg.AuthMode == "cloudflare" {
		if cfg.CloudflareIssuer == "" || cfg.CloudflareAudience == "" || cfg.AdminEmail == "" {
			return Config{}, errors.New("cloudflare auth requires THORSYNC_CF_ISSUER, THORSYNC_CF_AUDIENCE[_FILE], and THORSYNC_ADMIN_EMAIL[_FILE]")
		}
		if cfg.CloudflareJWKSURL == "" {
			cfg.CloudflareJWKSURL = cfg.CloudflareIssuer + "/cdn-cgi/access/certs"
		}
	}
	if cfg.SoftQuotaBytes <= 0 || cfg.ReserveBytes < 0 {
		return Config{}, errors.New("archive quota values must be positive")
	}
	return cfg, nil
}

func (c Config) DatabasePath() string { return filepath.Join(c.DataDir, "thorsync.db") }
func (c Config) ArtworkDir() string   { return filepath.Join(c.DataDir, "artwork") }

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func secret(key string) (string, error) {
	if filename := strings.TrimSpace(os.Getenv(key + "_FILE")); filename != "" {
		value, err := os.ReadFile(filename)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", key, err)
		}
		return strings.TrimSpace(string(value)), nil
	}
	return strings.TrimSpace(os.Getenv(key)), nil
}

func envInt64(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
