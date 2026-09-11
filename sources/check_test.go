package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"chel74alert/config"
)

func TestCheckFeedsRequiresAtLeastOne(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, rss(item("x", "Беспилотная опасность в Челябинской области", "Fri, 11 Sep 2026 10:32:00 +0500")))
	}))
	t.Cleanup(okSrv.Close)
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(badSrv.Close)

	cfg := config.Defaults()
	cfg.Feeds = []config.Feed{
		{Name: "bad", URL: badSrv.URL},
		{Name: "ok", URL: okSrv.URL},
	}
	if err := CheckFeeds(context.Background(), cfg); err != nil {
		t.Fatalf("одна живая лента достаточна: %v", err)
	}

	cfg.Feeds = []config.Feed{{Name: "bad", URL: badSrv.URL}}
	if err := CheckFeeds(context.Background(), cfg); err == nil {
		t.Fatal("если все ленты мертвы, preflight должен падать")
	}

	if err := CheckFeeds(context.Background(), config.Config{}); err == nil {
		t.Fatal("пустой список лент — ошибка")
	}
}
