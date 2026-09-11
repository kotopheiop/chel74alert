package sources

import (
	"regexp"
	"strings"
	"unicode"

	"chel74alert/config"
	"chel74alert/models"
)

var htmlTagRe = regexp.MustCompile(`(?i)<[^>]+>`)

var clearVerbs = []string{"снят", "отмен", "заверш"}
var modeWords = []string{"режим", "опасност"}

func Classify(cfg config.Config, title, summary string, regionImplicit bool) (models.Kind, bool) {
	text := normalize(title + " " + stripHTML(summary))
	if !regionImplicit && !containsAny(text, cfg.RegionKeywords) {
		return "", false
	}
	if isInvestigation(text) && !hasAlertSpeech(text) {
		return "", false
	}
	clear := isAllClear(text, cfg.ClearKeywords)
	if !clear && !isOfficialAlert(text, cfg.ThreatKeywords) {
		return "", false
	}
	if clear {
		return models.KindClear, true
	}
	return models.KindDanger, true
}

func isOfficialAlert(text string, threat []string) bool {
	if hasNearby(text, threat, []string{"опасност"}, 2) {
		return true
	}
	return hasNearby(text, []string{"режим"}, threat, 3)
}

func hasAlertSpeech(text string) bool {
	return containsAny(text, []string{"режим", "отбой", "объяв", "введен", "введён", "ввели", "угроза миновала"})
}

func isInvestigation(text string) bool {
	return containsAny(text, []string{
		"фсб",
		"тайник",
		"диверсант",
		"задержа",
		"накрыл",
		"накрыла",
		"изъял",
		"изъят",
		"уголовн",
		"следственн",
		"псевдо",
	})
}

func hasNearby(text string, left, right []string, maxGap int) bool {
	toks := tokens(text)
	for i, tok := range toks {
		if !tokenMatchesAny(tok, left) {
			continue
		}
		for d := 1; d <= maxGap+1 && i+d < len(toks); d++ {
			if tokenMatchesAny(toks[i+d], right) {
				return true
			}
		}
		for d := 1; d <= maxGap+1 && i-d >= 0; d++ {
			if tokenMatchesAny(toks[i-d], right) {
				return true
			}
		}
	}
	return false
}

func tokenMatchesAny(tok string, needles []string) bool {
	for _, n := range needles {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		if tok == n || strings.HasPrefix(tok, n) {
			return true
		}
	}
	return false
}

func isAllClear(text string, phrases []string) bool {
	if containsAny(text, phrases) {
		return true
	}
	return hasAdjacentClear(text)
}

func hasAdjacentClear(text string) bool {
	toks := tokens(text)
	for i, tok := range toks {
		if isModeWord(tok) {
			if i+1 < len(toks) && isClearVerb(toks[i+1]) {
				return true
			}
		}
		if isClearVerb(tok) {
			if i+1 < len(toks) && isModeWord(toks[i+1]) {
				return true
			}
			if i+2 < len(toks) && isModeWord(toks[i+2]) {
				return true
			}
		}
	}
	return false
}

func isModeWord(tok string) bool {
	for _, m := range modeWords {
		if tok == m || strings.HasPrefix(tok, m) {
			return true
		}
	}
	return false
}

func isClearVerb(tok string) bool {
	for _, v := range clearVerbs {
		if tok == v || strings.HasPrefix(tok, v) {
			return true
		}
	}
	return false
}

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}

func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func tokens(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		if strings.Contains(n, " ") {
			if strings.Contains(text, n) {
				return true
			}
			continue
		}
		if containsTokenPrefix(text, n) {
			return true
		}
	}
	return false
}

func containsTokenPrefix(text, needle string) bool {
	for _, tok := range tokens(text) {
		if tok == needle || strings.HasPrefix(tok, needle) {
			return true
		}
	}
	return false
}
