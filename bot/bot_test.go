package bot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"chel74alert/models"
	"chel74alert/store"
)

type tgMsg struct {
	ChatID string
	Text   string
}

func newTestBot(t *testing.T, st *store.Store) (*Bot, *[]tgMsg) {
	t.Helper()
	return newTestBotCfg(t, st, testBotCfg{memberStatus: "administrator"})
}

type testBotCfg struct {
	memberStatus string
	forbidden    bool
}

func newTestBotCfg(t *testing.T, st *store.Store, cfg testBotCfg) (*Bot, *[]tgMsg) {
	t.Helper()
	t.Cleanup(func() { _ = st.Close() })
	if cfg.memberStatus == "" {
		cfg.memberStatus = "member"
	}
	var mu sync.Mutex
	var sent []tgMsg

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"74. Тревога","username":"chel74_alert_bot"}}`)
		case strings.HasSuffix(r.URL.Path, "/getChatMember"):
			_ = r.ParseForm()
			_, _ = io.WriteString(w, `{"ok":true,"result":{"user":{"id":1,"is_bot":false,"first_name":"u"},"status":"`+cfg.memberStatus+`"}}`)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_ = r.ParseForm()
			if cfg.forbidden {
				_, _ = io.WriteString(w, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`)
				return
			}
			mu.Lock()
			sent = append(sent, tgMsg{ChatID: r.Form.Get("chat_id"), Text: r.Form.Get("text")})
			mu.Unlock()
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"ok"}}`)
		default:
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
		}
	}))
	t.Cleanup(srv.Close)

	api, err := tgbotapi.NewBotAPIWithClient("TESTTOKEN", srv.URL+"/bot%s/%s", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return &Bot{api: api, store: st}, &sent
}

func cmd(chatID int64, command string) tgbotapi.Update {
	text := "/" + command
	return tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat:      &tgbotapi.Chat{ID: chatID, Type: "private"},
			Text:      text,
			Entities:  []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(text)}},
		},
	}
}

func TestStartSendsActiveDanger(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alert := models.Alert{
		ID:        "today",
		Title:     "Беспилотную опасность объявили в Челябинской области",
		Source:    "74.ru",
		URL:       "https://74.ru/x",
		Published: time.Date(2026, 9, 11, 10, 32, 0, 0, time.Local),
		Kind:      models.KindDanger,
	}
	if err := st.RecordNotify(alert); err != nil {
		t.Fatal(err)
	}

	b, sent := newTestBot(t, st)
	b.handle(cmd(152823378, "start"))

	if !st.IsSubscribed(152823378) {
		t.Fatal("после /start должна быть подписка")
	}
	if len(*sent) != 2 {
		t.Fatalf("welcome + тревога, получили %d: %s", len(*sent), dump(*sent))
	}
	if !strings.Contains((*sent)[0].Text, "Вы подписались") {
		t.Fatalf("первым должно быть приветствие: %s", (*sent)[0].Text)
	}
	if !strings.Contains((*sent)[1].Text, "🚨 ВНИМАНИЕ") {
		t.Fatalf("второй — действующая тревога: %s", (*sent)[1].Text)
	}
	if !strings.Contains((*sent)[1].Text, alert.Title) {
		t.Fatalf("нет заголовка тревоги: %s", (*sent)[1].Text)
	}
}

func TestStartDoesNotSendAfterAllClear(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecordNotify(models.Alert{ID: "c", Title: "снят", Kind: models.KindClear}); err != nil {
		t.Fatal(err)
	}
	b, sent := newTestBot(t, st)
	b.handle(cmd(1, "start"))
	if len(*sent) != 1 || !strings.Contains((*sent)[0].Text, "Вы подписались") {
		t.Fatalf("после отбоя только приветствие: %s", dump(*sent))
	}
}

func TestStatusHidesSubscriberCount(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Subscribe(1)
	_ = st.Subscribe(2)
	_ = st.RecordNotify(models.Alert{
		ID:    "d",
		Title: "Беспилотную опасность объявили в Челябинской области",
		Kind:  models.KindDanger,
	})

	b, sent := newTestBot(t, st)
	b.handle(cmd(1, "status"))
	if len(*sent) != 1 {
		t.Fatalf("msgs=%s", dump(*sent))
	}
	text := (*sent)[0].Text
	if strings.Contains(text, "Подписчиков") {
		t.Fatalf("нельзя светить число подписчиков: %s", text)
	}
	if !strings.Contains(text, "подписка активна") || !strings.Contains(text, "действует беспилотная опасность") {
		t.Fatalf("status: %s", text)
	}
}

func TestStatusShowsAllClearTime(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Subscribe(1)
	cleared := time.Date(2026, 9, 11, 12, 40, 0, 0, time.Local)
	if err := st.RecordNotify(models.Alert{
		ID:        "c",
		Title:     "режим снят",
		Kind:      models.KindClear,
		Published: cleared,
	}); err != nil {
		t.Fatal(err)
	}

	b, sent := newTestBot(t, st)
	b.handle(cmd(1, "status"))
	text := (*sent)[0].Text
	if !strings.Contains(text, "тревоги нет") {
		t.Fatalf("нет строки про отсутствие тревоги: %s", text)
	}
	if !strings.Contains(text, "Отбой: 11.09.2026 12:40") {
		t.Fatalf("нет времени отбоя: %s", text)
	}
}

func TestNotifyOnlySubscribers(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Subscribe(10)
	_ = st.Subscribe(20)
	_ = st.Unsubscribe(20)

	b, sent := newTestBot(t, st)
	b.Notify(context.Background(), models.Alert{ID: "d", Title: "Беспилотная опасность в Челябинской области", Kind: models.KindDanger})
	if len(*sent) != 1 || (*sent)[0].ChatID != "10" {
		t.Fatalf("только подписчик 10: %s", dump(*sent))
	}
	if !strings.Contains((*sent)[0].Text, "🚨") {
		t.Fatalf("текст тревоги: %s", (*sent)[0].Text)
	}
}

func TestStopUnsubscribes(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newTestBot(t, st)
	b.handle(cmd(7, "start"))
	b.handle(cmd(7, "stop"))
	if st.IsSubscribed(7) {
		t.Fatal("после /stop подписки быть не должно")
	}
}

func TestGroupAutoSubscribeOnAdd(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.RecordNotify(models.Alert{ID: "d", Title: "Беспилотная опасность в Челябинской области", Kind: models.KindDanger})

	b, sent := newTestBot(t, st)
	chatID := int64(-100123)
	b.handle(tgbotapi.Update{
		MyChatMember: &tgbotapi.ChatMemberUpdated{
			Chat: tgbotapi.Chat{ID: chatID, Type: "supergroup", Title: "Семья"},
			NewChatMember: tgbotapi.ChatMember{
				User:   &tgbotapi.User{ID: b.api.Self.ID, IsBot: true},
				Status: "member",
			},
		},
	})

	if !st.IsSubscribed(chatID) {
		t.Fatal("группа должна подписаться без /start")
	}
	if len(*sent) != 2 {
		t.Fatalf("приветствие чата + тревога: %s", dump(*sent))
	}
	if !strings.Contains((*sent)[0].Text, "Этот чат подписан") {
		t.Fatalf("текст для чата: %s", (*sent)[0].Text)
	}

	b.handle(tgbotapi.Update{
		MyChatMember: &tgbotapi.ChatMemberUpdated{
			Chat: tgbotapi.Chat{ID: chatID, Type: "supergroup"},
			NewChatMember: tgbotapi.ChatMember{
				User:   &tgbotapi.User{ID: b.api.Self.ID, IsBot: true},
				Status: "member",
			},
		},
	})
	if len(*sent) != 2 {
		t.Fatalf("повторное добавление не должно спамить: %s", dump(*sent))
	}
}

func TestGroupUnsubscribeOnKick(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newTestBot(t, st)
	chatID := int64(-50)
	_ = st.Subscribe(chatID)
	b.handle(tgbotapi.Update{
		MyChatMember: &tgbotapi.ChatMemberUpdated{
			Chat: tgbotapi.Chat{ID: chatID, Type: "group"},
			NewChatMember: tgbotapi.ChatMember{
				User:   &tgbotapi.User{ID: b.api.Self.ID, IsBot: true},
				Status: "kicked",
			},
		},
	})
	if st.IsSubscribed(chatID) {
		t.Fatal("после удаления бота чат должен отписаться")
	}
}

func TestPrivateMembershipDoesNotAutoSubscribe(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, sent := newTestBot(t, st)
	b.handle(tgbotapi.Update{
		MyChatMember: &tgbotapi.ChatMemberUpdated{
			Chat: tgbotapi.Chat{ID: 99, Type: "private"},
			NewChatMember: tgbotapi.ChatMember{
				User:   &tgbotapi.User{ID: b.api.Self.ID, IsBot: true},
				Status: "member",
			},
		},
	})
	if st.IsSubscribed(99) {
		t.Fatal("личка без /start не подписывается")
	}
	if len(*sent) != 0 {
		t.Fatalf("не ждали сообщений: %s", dump(*sent))
	}
}

func TestGroupSubscribeViaNewChatMembers(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newTestBot(t, st)
	chatID := int64(-77)
	b.handle(tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID, Type: "group"},
			NewChatMembers: []tgbotapi.User{
				{ID: b.api.Self.ID, IsBot: true, UserName: "chel74_alert_bot"},
			},
		},
	})
	if !st.IsSubscribed(chatID) {
		t.Fatal("добавление бота в new_chat_members должно подписать чат")
	}
}

func TestGroupStopRequiresAdmin(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chatID := int64(-100)
	_ = st.Subscribe(chatID)
	b, sent := newTestBotCfg(t, st, testBotCfg{memberStatus: "member"})
	upd := cmd(chatID, "stop")
	upd.Message.Chat.Type = "supergroup"
	upd.Message.From = &tgbotapi.User{ID: 42, FirstName: "u"}
	b.handle(upd)
	if !st.IsSubscribed(chatID) {
		t.Fatal("участник не должен отписать чат")
	}
	if len(*sent) != 1 || !strings.Contains((*sent)[0].Text, "администратор") {
		t.Fatalf("отказ: %s", dump(*sent))
	}
}

func TestGroupStopByAdmin(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chatID := int64(-101)
	_ = st.Subscribe(chatID)
	b, sent := newTestBotCfg(t, st, testBotCfg{memberStatus: "administrator"})
	upd := cmd(chatID, "stop")
	upd.Message.Chat.Type = "supergroup"
	upd.Message.From = &tgbotapi.User{ID: 7, FirstName: "admin"}
	b.handle(upd)
	if st.IsSubscribed(chatID) {
		t.Fatal("админ должен отписать чат")
	}
	if len(*sent) != 1 || !strings.Contains((*sent)[0].Text, "отписались") {
		t.Fatalf("stop: %s", dump(*sent))
	}
}

func TestNotifyUnsubscribesBlockedChat(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Subscribe(10)
	b, _ := newTestBotCfg(t, st, testBotCfg{forbidden: true})
	if err := b.Notify(context.Background(), models.Alert{ID: "d", Title: "тревога", Kind: models.KindDanger}); err == nil {
		t.Fatal("ожидал ошибку 403")
	}
	if st.IsSubscribed(10) {
		t.Fatal("заблокировавший бота чат должен отписаться")
	}
}

func dump(msgs []tgMsg) string {
	raw, _ := json.Marshal(msgs)
	return string(raw)
}
