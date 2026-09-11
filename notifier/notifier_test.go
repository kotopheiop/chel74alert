package notifier

import (
	"strings"
	"testing"
	"time"

	"chel74alert/models"
)

func TestFormatDangerAndClear(t *testing.T) {
	published := time.Date(2026, 9, 11, 10, 32, 0, 0, time.Local)
	danger := Format(models.Alert{
		Title:     "Беспилотную опасность объявили в Челябинской области",
		Source:    "74.ru",
		URL:       "https://74.ru/text/incidents/2026/09/11/76621117/",
		Published: published,
		Kind:      models.KindDanger,
	})
	for _, need := range []string{
		"🚨 ВНИМАНИЕ: опасность БПЛА/ракет",
		"<b>Беспилотную опасность объявили в Челябинской области</b>",
		"Источник: 74.ru",
		"11.09.2026 10:32",
		"https://74.ru/text/incidents/2026/09/11/76621117/",
		"112",
	} {
		if !strings.Contains(danger, need) {
			t.Fatalf("нет %q в:\n%s", need, danger)
		}
	}

	clear := Format(models.Alert{Title: "режим снят", Kind: models.KindClear})
	if !strings.Contains(clear, "✅ Отбой опасности БПЛА/ракет") {
		t.Fatalf("отбой: %s", clear)
	}
	if !strings.Contains(clear, "время неизвестно") {
		t.Fatalf("пустое время: %s", clear)
	}
}

func TestThreatName(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Беспилотную опасность объявили в Челябинской области", "беспилотная опасность"},
		{"В Челябинской области объявлена ракетная опасность", "ракетная опасность"},
		{"Беспилотная и ракетная опасность в Челябинске", "беспилотная и ракетная опасность"},
		{"Режим повышенной готовности", "опасность БПЛА/ракет"},
	}
	for _, tt := range tests {
		got := ThreatName(models.Alert{Title: tt.title})
		if got != tt.want {
			t.Fatalf("%q: got %q want %q", tt.title, got, tt.want)
		}
	}
}

func TestFormatEscapesHTML(t *testing.T) {
	got := Format(models.Alert{
		Title:  `<script>alert("x")</script>`,
		Source: "A&B",
		URL:    "https://example.com/?q=1&x=2",
		Kind:   models.KindDanger,
	})
	if strings.Contains(got, "<script>") {
		t.Fatal("заголовок должен быть экранирован")
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("ожидал escape: %s", got)
	}
	if !strings.Contains(got, "A&amp;B") {
		t.Fatalf("источник: %s", got)
	}
}
