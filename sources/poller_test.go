package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"chel74alert/config"
	"chel74alert/models"
	"chel74alert/store"
)

func testPoller(t *testing.T, rssBody string, cooldown time.Duration) (*Poller, *store.Store, <-chan models.Alert, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, rssBody)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.BackfillHours = 6
	cfg.EventCooldown = cooldown
	cfg.Feeds = []config.Feed{{Name: "test", URL: srv.URL, RegionImplicit: false}}

	out := make(chan models.Alert, 16)
	p := &Poller{
		cfg:    cfg,
		store:  st,
		parser: gofeed.NewParser(),
		out:    out,
	}
	p.parser.Client = srv.Client()
	t.Cleanup(func() { _ = st.Close() })
	return p, st, out, dir
}

func drain(ch <-chan models.Alert) []models.Alert {
	var out []models.Alert
	for {
		select {
		case a := <-ch:
			out = append(out, a)
		default:
			return out
		}
	}
}

func rss(items string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>t</title>` + items + `</channel></rss>`
}

func item(guid, title, pub string) string {
	return fmt.Sprintf(`<item><guid>%s</guid><title>%s</title><link>https://example.com/%s</link><pubDate>%s</pubDate><description>%s</description></item>`,
		guid, title, guid, pub, title)
}

func TestPollerEmitsTodayDangerOnce(t *testing.T) {
	now := time.Now()
	today := now.Format(time.RFC1123Z)
	old := now.Add(-48 * time.Hour).Format(time.RFC1123Z)

	body := rss(
		item("old", "Беспилотная опасность объявлена в Челябинской области 9 сентября", old) +
			item("a", "Беспилотную опасность объявили в Челябинской области", today) +
			item("b", "Беспилотная опасность объявлена в Челябинской области 11 сентября", today) +
			item("fire", "Пожар в Челябинске", today),
	)
	p, st, out, _ := testPoller(t, body, 90*time.Minute)
	p.poll(context.Background())

	got := drain(out)
	if len(got) != 1 {
		t.Fatalf("ожидал 1 оповещение за волну, получил %d: %+v", len(got), got)
	}
	if got[0].Kind != models.KindDanger {
		t.Fatalf("kind=%q", got[0].Kind)
	}
	if st.ActiveDanger() == nil {
		t.Fatal("после тревоги ActiveDanger должен быть задан")
	}

	p.poll(context.Background())
	if extra := drain(out); len(extra) != 0 {
		t.Fatalf("повторный опрос не должен слать то же самое: %+v", extra)
	}
}

func TestPollerEmitsAllClearAfterDanger(t *testing.T) {
	now := time.Now()
	t1 := now.Add(-30 * time.Minute).Format(time.RFC1123Z)
	t2 := now.Format(time.RFC1123Z)

	body1 := rss(item("d", "Беспилотная опасность в Челябинской области", t1))
	p, st, out, _ := testPoller(t, body1, 90*time.Minute)
	p.poll(context.Background())
	if got := drain(out); len(got) != 1 || got[0].Kind != models.KindDanger {
		t.Fatalf("сначала тревога: %+v", got)
	}

	body2 := rss(
		item("d", "Беспилотная опасность в Челябинской области", t1) +
			item("c", "В Челябинской области режим беспилотная опасность снят", t2),
	)
	p.cfg.Feeds[0].URL = rewriteFeed(t, p, body2)
	p.poll(context.Background())
	got := drain(out)
	if len(got) != 1 || got[0].Kind != models.KindClear {
		t.Fatalf("должен уйти отбой: %+v", got)
	}
	if st.ActiveDanger() != nil {
		t.Fatal("после отбоя ActiveDanger должен быть пуст")
	}
}

func TestPollerIgnoresFailedFeedAndReadsOther(t *testing.T) {
	now := time.Now().Format(time.RFC1123Z)
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, rss(item("x", "Беспилотная опасность в Челябинской области", now)))
	}))
	t.Cleanup(ok.Close)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(bad.Close)

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Defaults()
	cfg.EventCooldown = time.Minute
	cfg.Feeds = []config.Feed{
		{Name: "bad", URL: bad.URL, RegionImplicit: false},
		{Name: "ok", URL: ok.URL, RegionImplicit: false},
	}
	out := make(chan models.Alert, 4)
	p := &Poller{cfg: cfg, store: st, parser: gofeed.NewParser(), out: out}
	p.parser.Client = http.DefaultClient
	p.poll(context.Background())
	got := drain(out)
	if len(got) != 1 {
		t.Fatalf("рабочая лента должна пробиться через ошибку другой: %+v", got)
	}
}

func TestHydrateLastAfterRestart(t *testing.T) {
	now := time.Now().Format(time.RFC1123Z)
	body := rss(item("a", "Беспилотная опасность в Челябинской области", now))
	p, _, out, dir := testPoller(t, body, 90*time.Minute)
	p.poll(context.Background())
	_ = drain(out)
	if err := p.store.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st2.LastAlert() == nil || st2.LastAlert().Kind != models.KindDanger {
		t.Fatal("последняя тревога должна пережить перезапуск")
	}
	t.Cleanup(func() { _ = st2.Close() })

	if err := st2.HydrateLast(models.Alert{ID: "should-not", Kind: models.KindDanger, Title: "nope"}); err != nil {
		t.Fatal(err)
	}
	if st2.LastAlert().ID == "should-not" {
		t.Fatal("HydrateLast не должен затирать уже сохранённую тревогу")
	}
}

func rewriteFeed(t *testing.T, p *Poller, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	p.parser.Client = srv.Client()
	return srv.URL
}

func TestRecentDropsStaleAlerts(t *testing.T) {
	cfg := config.Defaults()
	cfg.BackfillHours = 6
	p := &Poller{cfg: cfg}
	old := models.Alert{ID: "old", Title: "old", Published: time.Now().Add(-48 * time.Hour), Kind: models.KindDanger}
	fresh := models.Alert{ID: "new", Title: "new", Published: time.Now(), Kind: models.KindDanger}
	got := p.recent([]models.Alert{old, fresh})
	if len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("в окно должны попасть только свежие: %+v", got)
	}
}
