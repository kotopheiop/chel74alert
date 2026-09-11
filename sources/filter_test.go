package sources

import (
	"testing"

	"chel74alert/config"
	"chel74alert/models"
)

func TestClassify(t *testing.T) {
	cfg := config.Defaults()

	tests := []struct {
		name           string
		title, summary string
		regionImplicit bool
		wantOK         bool
		wantKind       models.Kind
	}{
		{
			name:     "сегодняшняя тревога URA",
			title:    "Беспилотная опасность объявлена в Челябинской области 11 сентября - URA.RU",
			wantOK:   true,
			wantKind: models.KindDanger,
		},
		{
			name:     "тревога 74.ru",
			title:    "Беспилотную опасность объявили в Челябинской области",
			summary:  "Власти предупредили о возможных перебоях со связью",
			wantOK:   true,
			wantKind: models.KindDanger,
		},
		{
			name:     "ракетная опасность",
			title:    "В Челябинской области объявлена ракетная опасность",
			wantOK:   true,
			wantKind: models.KindDanger,
		},
		{
			name:     "отбой",
			title:    "ВНИМАНИЕ! На территории Челябинской области режим беспилотная опасность снят",
			wantOK:   true,
			wantKind: models.KindClear,
		},
		{
			name:     "отбой формулировкой завершился",
			title:    "Режим беспилотной опасности завершился в Челябинской области",
			wantOK:   true,
			wantKind: models.KindClear,
		},
		{
			name:   "другой регион без Челябинска",
			title:  "Пермь атакуют беспилотники: что в это время происходит в соседних регионах",
			wantOK: false,
		},
		{
			name:           "пожар МЧС",
			title:          "Пожар в городе Магнитогорск",
			summary:        "загорание контейнера для сбора бытовых отходов",
			regionImplicit: true,
			wantOK:         false,
		},
		{
			name:           "БПЛА в ленте МЧС без слова Челябинск",
			title:          "ВНИМАНИЕ! Объявлен режим беспилотная опасность",
			regionImplicit: true,
			wantOK:         true,
			wantKind:       models.KindDanger,
		},
		{
			name:     "HTML в описании",
			title:    "Опасность БПЛА",
			summary:  `<p>В <b>Челябинской</b> области&nbsp;объявлена беспилотная опасность</p>`,
			wantOK:   true,
			wantKind: models.KindDanger,
		},
		{
			name:   "без ключевых слов угрозы",
			title:  "В Челябинске ограничат движение в День города",
			wantOK: false,
		},
		{
			name:     "Озёрск через ё",
			title:    "Беспилотная опасность в Озёрске",
			wantOK:   true,
			wantKind: models.KindDanger,
		},
		{
			name:   "аша не срабатывает внутри наша",
			title:  "Наша редакция: беспилотники атаковали Пермь",
			wantOK: false,
		},
		{
			name:     "город Аша",
			title:    "Беспилотная опасность в Аше",
			wantOK:   true,
			wantKind: models.KindDanger,
		},
		{
			name:     "губернатор снят — не отбой",
			title:    "Беспилотная опасность в Челябинской области, губернатор снят",
			wantOK:   true,
			wantKind: models.KindDanger,
		},
		{
			name:   "отмена мероприятий из-за БПЛА без режима — не тревога",
			title:  "В Челябинске отмена мероприятий из-за БПЛА",
			wantOK: false,
		},
		{
			name:   "ФСБ и тайник с дронами — не режим опасности",
			title:  "«Террористы и тайник с дронами»: под Челябинском ФСБ накрыла группу псевдо-диверсантов. Фото",
			wantOK: false,
		},
		{
			name:   "задержание с дроном",
			title:  "В Челябинске задержали мужчину с дроном",
			wantOK: false,
		},
		{
			name:   "фестиваль дронов",
			title:  "Фестиваль дронов прошёл в Челябинске",
			wantOK: false,
		},
		{
			name:     "снята беспилотная опасность",
			title:    "Снята беспилотная опасность в Челябинской области",
			wantOK:   true,
			wantKind: models.KindClear,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ok := Classify(cfg, tt.title, tt.summary, tt.regionImplicit)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v (kind=%q)", ok, tt.wantOK, kind)
			}
			if tt.wantOK && kind != tt.wantKind {
				t.Fatalf("kind=%q, want %q", kind, tt.wantKind)
			}
		})
	}
}
