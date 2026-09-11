package sources

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"chel74alert/config"
	"chel74alert/models"
	"chel74alert/store"
)

const userAgent = "Mozilla/5.0 (compatible; Chel74Alert/1.0; Chelyabinsk UAV/missile RSS monitor)"

type Poller struct {
	cfg    config.Config
	store  *store.Store
	parser *gofeed.Parser
	out    chan<- models.Alert
}

func Start(ctx context.Context, cfg config.Config, st *store.Store, out chan<- models.Alert) {
	p := &Poller{
		cfg:    cfg,
		store:  st,
		parser: gofeed.NewParser(),
		out:    out,
	}
	p.parser.Client = rssClient()

	p.poll(ctx)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	var found []models.Alert
	for _, feed := range p.cfg.Feeds {
		if ctx.Err() != nil {
			return
		}
		items, err := p.fetch(ctx, feed)
		if err != nil {
			log.Printf("rss %s: %v", feed.Name, err)
			continue
		}
		found = append(found, items...)
	}

	ids := make([]string, 0, len(found))
	byID := make(map[string]models.Alert, len(found))
	for _, a := range found {
		ids = append(ids, a.ID)
		byID[a.ID] = a
	}

	unseenIDs := p.store.FilterNew(ids)
	unseen := make([]models.Alert, 0, len(unseenIDs))
	for _, id := range unseenIDs {
		unseen = append(unseen, byID[id])
	}
	fresh := p.recent(unseen)
	if len(fresh) > 0 {
		log.Printf("новых оповещений в окне %d ч: %d", p.cfg.BackfillHours, len(fresh))
	}

	sort.SliceStable(fresh, func(i, j int) bool {
		if fresh[i].Published.Equal(fresh[j].Published) {
			return fresh[i].Title < fresh[j].Title
		}
		return fresh[i].Published.Before(fresh[j].Published)
	})

	toSend := p.store.NextToNotify(fresh, p.cfg.EventCooldown)
	if skipped := len(fresh) - len(toSend); skipped > 0 {
		log.Printf("пропуск дублей той же волны: %d", skipped)
	}

	var hydrate *models.Alert
	if p.store.LastAlert() == nil {
		hydrate = p.hydrateCandidate(found)
	}
	if err := p.store.CommitPoll(ids, toSend, hydrate); err != nil {
		log.Printf("store: %v", err)
		return
	}

	for _, a := range toSend {
		select {
		case <-ctx.Done():
			return
		case p.out <- a:
		}
	}
}

func (p *Poller) recent(alerts []models.Alert) []models.Alert {
	if p.cfg.BackfillHours <= 0 {
		return alerts
	}
	cutoff := time.Now().Add(-time.Duration(p.cfg.BackfillHours) * time.Hour)
	var out []models.Alert
	for _, a := range alerts {
		if a.Published.IsZero() || !a.Published.Before(cutoff) {
			out = append(out, a)
		}
	}
	return out
}

func (p *Poller) hydrateCandidate(found []models.Alert) *models.Alert {
	window := p.recent(found)
	var best *models.Alert
	for i := range window {
		a := window[i]
		if best == nil || a.Published.After(best.Published) {
			cp := a
			best = &cp
		}
	}
	return best
}

func (p *Poller) fetch(ctx context.Context, feed config.Feed) ([]models.Alert, error) {
	fp, err := p.parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		return nil, err
	}

	var out []models.Alert
	for _, item := range fp.Items {
		if item == nil {
			continue
		}
		kind, ok := Classify(p.cfg, item.Title, item.Description+" "+item.Content, feed.RegionImplicit)
		if !ok {
			continue
		}
		id := item.GUID
		if id == "" {
			id = item.Link
		}
		if id == "" {
			id = feed.Name + "|" + item.Title
		}
		published := time.Time{}
		if item.PublishedParsed != nil {
			published = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			published = *item.UpdatedParsed
		}
		out = append(out, models.Alert{
			ID:        id,
			Title:     strings.TrimSpace(item.Title),
			Summary:   strings.TrimSpace(stripHTML(item.Description)),
			URL:       item.Link,
			Source:    feed.Name,
			Published: published,
			Kind:      kind,
		})
	}
	return out, nil
}

func rssClient() *http.Client {
	return &http.Client{
		Timeout: 25 * time.Second,
		Transport: uaTransport{
			base: http.DefaultTransport,
			ua:   userAgent,
		},
	}
}

type uaTransport struct {
	base http.RoundTripper
	ua   string
}

func (t uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", t.ua)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	return t.base.RoundTrip(req)
}
