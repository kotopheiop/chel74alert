package sources

import (
	"context"
	"fmt"
	"log"

	"github.com/mmcdole/gofeed"

	"chel74alert/config"
)

func CheckFeeds(ctx context.Context, cfg config.Config) error {
	if len(cfg.Feeds) == 0 {
		return fmt.Errorf("список RSS-лент пуст")
	}

	parser := gofeed.NewParser()
	parser.Client = rssClient()

	ok := 0
	var lastErr error
	for _, feed := range cfg.Feeds {
		fp, err := parser.ParseURLWithContext(feed.URL, ctx)
		if err != nil {
			log.Printf("preflight RSS %s: ошибка: %v", feed.Name, err)
			lastErr = err
			continue
		}
		matched := 0
		for _, item := range fp.Items {
			if item == nil {
				continue
			}
			if _, yes := Classify(cfg, item.Title, item.Description+" "+item.Content, feed.RegionImplicit); yes {
				matched++
			}
		}
		log.Printf("preflight RSS %s: ок, записей %d, по региону/угрозе %d", feed.Name, len(fp.Items), matched)
		ok++
	}
	if ok == 0 {
		if lastErr != nil {
			return fmt.Errorf("ни одна RSS-лента не читается: %w", lastErr)
		}
		return fmt.Errorf("ни одна RSS-лента не читается")
	}
	if ok < len(cfg.Feeds) {
		log.Printf("preflight RSS: доступны %d из %d лент, бот стартует", ok, len(cfg.Feeds))
	}
	return nil
}
