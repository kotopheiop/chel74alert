package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"chel74alert/models"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	return openStoreDir(t, t.TempDir())
}

func openStoreDir(t *testing.T, dir string) *Store {
	t.Helper()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSubscribeUnsubscribePersists(t *testing.T) {
	dir := t.TempDir()
	st := openStoreDir(t, dir)
	if err := st.Subscribe(111); err != nil {
		t.Fatal(err)
	}
	if err := st.Subscribe(222); err != nil {
		t.Fatal(err)
	}
	if !st.IsSubscribed(111) || !st.IsSubscribed(222) {
		t.Fatal("оба должны быть подписаны")
	}
	if err := st.Unsubscribe(111); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2 := openStoreDir(t, dir)
	if st2.IsSubscribed(111) {
		t.Fatal("отписка должна сохраниться")
	}
	if !st2.IsSubscribed(222) {
		t.Fatal("вторая подписка должна сохраниться")
	}
	ids := st2.ListSubscribers()
	if len(ids) != 1 || ids[0] != 222 {
		t.Fatalf("подписчики: %v", ids)
	}
}

func TestOpenRejectsSecondProcess(t *testing.T) {
	dir := t.TempDir()
	_ = openStoreDir(t, dir)
	if _, err := Open(dir); err == nil {
		t.Fatal("второй процесс должен получить ошибку блокировки")
	}
}

func TestCooldownAndActiveDanger(t *testing.T) {
	st := openStore(t)
	danger := models.Alert{ID: "d1", Title: "тревога беспилотная опасность в Челябинской области", Kind: models.KindDanger, Published: time.Now()}
	clear := models.Alert{ID: "c1", Title: "отбой", Kind: models.KindClear, Published: time.Now()}

	if !st.ShouldNotify("danger", danger.Title, time.Hour) {
		t.Fatal("первая тревога должна уходить")
	}
	if err := st.RecordNotify(danger); err != nil {
		t.Fatal(err)
	}
	if st.ActiveDanger() == nil {
		t.Fatal("ожидал активную опасность")
	}
	if st.ShouldNotify("danger", "Беспилотную опасность объявили в Челябинской области", time.Hour) {
		t.Fatal("перепечатка той же волны нельзя")
	}
	if !st.ShouldNotify("clear", clear.Title, time.Hour) {
		t.Fatal("отбой — другая волна, должен уйти")
	}
	if err := st.RecordNotify(clear); err != nil {
		t.Fatal(err)
	}
	if st.ActiveDanger() != nil {
		t.Fatal("после отбоя опасности быть не должно")
	}
	if st.LastClearTime().IsZero() {
		t.Fatal("время отбоя должно сохраниться")
	}
}

func TestSimilarReprintAfterCooldownStillSkipped(t *testing.T) {
	st := openStore(t)
	st.LastNotifyKind = "danger"
	st.LastNotifyAt = time.Now().Add(-2 * time.Hour)
	st.Last = &models.Alert{Title: "Беспилотную опасность объявили в Челябинской области"}
	if st.ShouldNotify("danger", "Беспилотная опасность объявлена в Челябинской области 11 сентября", time.Hour) {
		t.Fatal("перепечатка после cooldown не должна уходить")
	}
	if !st.ShouldNotify("danger", "В Челябинской области объявлена ракетная опасность", time.Hour) {
		t.Fatal("другая угроза после cooldown должна уйти")
	}
}

func TestNoiseDoesNotBlockRealDanger(t *testing.T) {
	st := openStore(t)
	st.LastNotifyKind = "danger"
	st.LastNotifyAt = time.Now()
	st.Last = &models.Alert{
		Title: "«Террористы и тайник с дронами»: под Челябинском ФСБ накрыла группу псевдо-диверсантов. Фото",
		Kind:  models.KindDanger,
	}
	if st.ActiveDanger() != nil {
		t.Fatal("криминальный сюжет не должен считаться действующей тревогой")
	}
	if !st.ShouldNotify("danger", "Беспилотную опасность объявили в Челябинской области", time.Hour) {
		t.Fatal("шумное последнее сообщение не должно глушить настоящий режим")
	}
}

func TestFilterNewAndMarkSeen(t *testing.T) {
	st := openStore(t)
	if got := st.FilterNew([]string{"a", "b"}); len(got) != 2 {
		t.Fatalf("все новые: %v", got)
	}
	if err := st.MarkSeen([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	got := st.FilterNew([]string{"a", "b", "c"})
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("после seen: %v", got)
	}
}

func TestLastAlertCopyIsIndependent(t *testing.T) {
	st := openStore(t)
	a := models.Alert{ID: "1", Title: "x", Kind: models.KindDanger}
	if err := st.RecordNotify(a); err != nil {
		t.Fatal(err)
	}
	got := st.LastAlert()
	got.Title = "mutated"
	if st.LastAlert().Title != "x" {
		t.Fatal("наружная мутация не должна портить store")
	}
}

func TestCommitPollOutboxAndAck(t *testing.T) {
	st := openStore(t)
	a := models.Alert{ID: "n1", Title: "тревога", Kind: models.KindDanger}
	if err := st.CommitPoll([]string{"n1", "old"}, []models.Alert{a}, nil); err != nil {
		t.Fatal(err)
	}
	if len(st.PendingAlerts()) != 1 {
		t.Fatalf("pending=%v", st.PendingAlerts())
	}
	if len(st.FilterNew([]string{"n1", "old"})) != 0 {
		t.Fatal("GUID должны быть seen до доставки")
	}
	if err := st.AckPending("n1"); err != nil {
		t.Fatal(err)
	}
	if len(st.PendingAlerts()) != 0 {
		t.Fatal("после ack pending пуст")
	}
}

func TestSeenBoolJSONStillLoads(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"subscribers":{},"seen":{"old-guid":true},"seeded":true}`)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	st := openStoreDir(t, dir)
	if got := st.FilterNew([]string{"old-guid", "new"}); len(got) != 1 || got[0] != "new" {
		t.Fatalf("миграция seen: %v", got)
	}
}

func TestPruneOldSeen(t *testing.T) {
	st := openStore(t)
	st.Seen["ancient"] = time.Now().Add(-30 * 24 * time.Hour).Unix()
	st.Seen["fresh"] = time.Now().Unix()
	if err := st.MarkSeen([]string{"fresh"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Seen["ancient"]; ok {
		t.Fatal("просроченный GUID должен удалиться")
	}
}

func TestAlertJSONSnakeCase(t *testing.T) {
	st := openStore(t)
	if err := st.RecordNotify(models.Alert{ID: "x", Title: "t", Kind: models.KindDanger}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(st.path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	last, _ := doc["last_alert"].(map[string]any)
	if last["id"] != "x" || last["title"] != "t" {
		t.Fatalf("ожидал snake_case: %v", last)
	}
}
