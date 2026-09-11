package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Feed struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	RegionImplicit bool   `json:"region_implicit"`
}

type Config struct {
	Token          string
	Proxy          string
	ProxyUser      string
	ProxyPassword  string
	PollInterval   time.Duration
	BackfillHours  int
	EventCooldown  time.Duration
	DataDir        string
	HealthAddr     string
	Feeds          []Feed
	ThreatKeywords []string
	ClearKeywords  []string
	RegionKeywords []string
}

func Load() (Config, error) {
	loadDotEnv(".env")

	cfg := Defaults()

	if v := strings.TrimSpace(os.Getenv("POLL_INTERVAL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("POLL_INTERVAL %q: %w", v, err)
		}
		cfg.PollInterval = d
	}
	if v := strings.TrimSpace(os.Getenv("EVENT_COOLDOWN")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("EVENT_COOLDOWN %q: %w", v, err)
		}
		cfg.EventCooldown = d
	}
	if v := strings.TrimSpace(os.Getenv("BACKFILL_HOURS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("BACKFILL_HOURS %q: %w", v, err)
		}
		cfg.BackfillHours = n
	}
	if v := strings.TrimSpace(os.Getenv("DATA_DIR")); v != "" {
		cfg.DataDir = v
	}
	if v, ok := os.LookupEnv("HEALTH_ADDR"); ok {
		cfg.HealthAddr = strings.TrimSpace(v)
	}

	if v := strings.TrimSpace(os.Getenv("RSS_FEEDS_JSON")); v != "" {
		var feeds []Feed
		if err := json.Unmarshal([]byte(v), &feeds); err != nil {
			return Config{}, fmt.Errorf("RSS_FEEDS_JSON: %w", err)
		}
		if len(feeds) == 0 {
			return Config{}, fmt.Errorf("RSS_FEEDS_JSON: список лент пуст")
		}
		for i, f := range feeds {
			if strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.URL) == "" {
				return Config{}, fmt.Errorf("RSS_FEEDS_JSON: лента %d без name/url", i)
			}
			feeds[i].Name = strings.TrimSpace(f.Name)
			feeds[i].URL = strings.TrimSpace(f.URL)
		}
		cfg.Feeds = feeds
	}

	cfg.Token = strings.TrimSpace(os.Getenv("TG_BOT_TOKEN"))
	cfg.Proxy = strings.TrimSpace(os.Getenv("TG_PROXY"))
	cfg.ProxyUser = strings.TrimSpace(os.Getenv("TG_PROXY_USER"))
	cfg.ProxyPassword = os.Getenv("TG_PROXY_PASSWORD")
	return cfg, nil
}

func Defaults() Config {
	return Config{
		PollInterval:  60 * time.Second,
		BackfillHours: 6,
		EventCooldown: 90 * time.Minute,
		DataDir:       "data",
		HealthAddr:    "",
		Feeds: []Feed{
			{
				Name:           "URA.RU",
				URL:            "https://ura.news/rss",
				RegionImplicit: false,
			},
			{
				Name:           "74.ru",
				URL:            "https://74.ru/text/rss.xml",
				RegionImplicit: false,
			},
			{
				Name:           "МЧС Челябинской области",
				URL:            "https://74.mchs.gov.ru/deyatelnost/press-centr/vse_novosti/rss",
				RegionImplicit: true,
			},
		},
		ThreatKeywords: []string{
			"бпла",
			"бвс",
			"беспилотн",
			"дрон",
			"шахед",
			"ракетн",
			"ракета",
		},
		ClearKeywords: []string{
			"отбой",
			"угроза миновала",
		},
		RegionKeywords: []string{
			"челябинск",
			"южноурал",
			"магнитогорск",
			"миасс",
			"златоуст",
			"копейск",
			"озерск",
			"озёрск",
			"снежинск",
			"троицк",
			"сатка",
			"чебаркуль",
			"кыштым",
			"аша",
			"аше",
			"ашинск",
		},
	}
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, val)
	}
}
