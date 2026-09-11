package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultsKeywordsCoverUAVAndMissiles(t *testing.T) {
	cfg := Defaults()
	joined := strings.Join(cfg.ThreatKeywords, ",")
	for _, n := range []string{"бпла", "беспилотн", "ракетн", "ракета"} {
		if !strings.Contains(joined, n) {
			t.Fatalf("в ThreatKeywords нет %q", n)
		}
	}
	if cfg.BackfillHours != 6 || cfg.EventCooldown != 90*time.Minute {
		t.Fatalf("окна по умолчанию: hours=%d cooldown=%s", cfg.BackfillHours, cfg.EventCooldown)
	}
}

func TestLoadReadsEnv(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", "token-1")
	t.Setenv("TG_PROXY", "http://127.0.0.1:1")
	t.Setenv("TG_PROXY_USER", "u")
	t.Setenv("TG_PROXY_PASSWORD", "p")
	t.Setenv("POLL_INTERVAL", "15s")
	t.Setenv("BACKFILL_HOURS", "3")
	t.Setenv("EVENT_COOLDOWN", "10m")
	t.Setenv("DATA_DIR", "tmp-data")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "token-1" || cfg.ProxyUser != "u" || cfg.ProxyPassword != "p" {
		t.Fatalf("proxy/token: %+v", cfg)
	}
	if cfg.PollInterval != 15*time.Second || cfg.BackfillHours != 3 || cfg.EventCooldown != 10*time.Minute {
		t.Fatalf("тайминги: %+v", cfg)
	}
}

func TestLoadRejectsBadDuration(t *testing.T) {
	t.Setenv("POLL_INTERVAL", "90мин")
	if _, err := Load(); err == nil {
		t.Fatal("ожидал ошибку POLL_INTERVAL")
	}
}

func TestLoadFeedsJSON(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", "t")
	t.Setenv("RSS_FEEDS_JSON", `[{"name":"x","url":"https://example.com/rss","region_implicit":true}]`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Feeds) != 1 || cfg.Feeds[0].Name != "x" || !cfg.Feeds[0].RegionImplicit {
		t.Fatalf("feeds=%+v", cfg.Feeds)
	}
}

func TestLoadDotEnvDoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=fromfile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOO", "fromenv")
	loadDotEnv(path)
	if os.Getenv("FOO") != "fromenv" {
		t.Fatal("уже заданный env нельзя перетирать из файла")
	}
}
