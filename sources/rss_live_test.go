package sources

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"chel74alert/config"
)

func TestLiveURAHasChelyabinskUAV(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	parser := gofeed.NewParser()
	parser.Client = &http.Client{
		Timeout: 25 * time.Second,
		Transport: uaTransport{
			base: http.DefaultTransport,
			ua:   userAgent,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var feed config.Feed
	for _, f := range cfg.Feeds {
		if f.Name == "URA.RU" {
			feed = f
			break
		}
	}
	if feed.URL == "" {
		t.Fatal("нет фида URA.RU")
	}

	fp, err := parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		t.Fatalf("ura.news rss: %v", err)
	}

	matched := 0
	for _, item := range fp.Items {
		if item == nil {
			continue
		}
		if _, ok := Classify(cfg, item.Title, item.Description+" "+item.Content, false); ok {
			matched++
			if matched == 1 {
				t.Logf("пример: %s", item.Title)
			}
		}
	}
	if matched == 0 {
		t.Skip("в текущей ленте URA.RU нет сообщений про режим БПЛА/ракет по Челябинску")
	}
	t.Logf("совпадений: %d из %d", matched, len(fp.Items))
}
