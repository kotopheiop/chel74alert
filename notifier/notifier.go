package notifier

import (
	"fmt"
	"html"
	"strings"
	"time"

	"chel74alert/models"
)

func Format(a models.Alert) string {
	header := "🚨 ВНИМАНИЕ: опасность БПЛА/ракет"
	if a.Kind == models.KindClear {
		header = "✅ Отбой опасности БПЛА/ракет"
	}

	title := html.EscapeString(a.Title)
	source := html.EscapeString(a.Source)
	when := a.Published.In(time.Local).Format("02.01.2006 15:04")
	if a.Published.IsZero() {
		when = "время неизвестно"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n<b>%s</b>\n", header, title)
	fmt.Fprintf(&b, "\nИсточник: %s", source)
	fmt.Fprintf(&b, "\nВремя: %s", when)
	if a.URL != "" {
		fmt.Fprintf(&b, "\n%s", html.EscapeString(a.URL))
	}
	b.WriteString("\n\nОриентируйтесь на официальные сообщения РСЧС / МЧС. При угрозе: не снимайте ПВО и БВС, при обнаружении обломков звоните 112.")
	return b.String()
}

func ThreatName(a models.Alert) string {
	text := strings.ToLower(a.Title + " " + a.Summary)
	uav := strings.Contains(text, "бпла") ||
		strings.Contains(text, "бвс") ||
		strings.Contains(text, "беспилот") ||
		strings.Contains(text, "дрон") ||
		strings.Contains(text, "шахед")
	missile := strings.Contains(text, "ракет")
	switch {
	case uav && missile:
		return "беспилотная и ракетная опасность"
	case missile:
		return "ракетная опасность"
	case uav:
		return "беспилотная опасность"
	default:
		return "опасность БПЛА/ракет"
	}
}
