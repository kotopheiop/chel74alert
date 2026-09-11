package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"chel74alert/models"
	"chel74alert/notifier"
	"chel74alert/store"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	store   *store.Store
	secrets []string
}

func New(token string, st *store.Store, client *http.Client, secrets ...string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, client)
	if err != nil {
		return nil, fmt.Errorf("%s", telegramDialHint(err, append([]string{token}, secrets...)...))
	}
	return &Bot{api: api, store: st, secrets: secrets}, nil
}

func telegramDialHint(err error, secrets ...string) string {
	msg := Redact(err, secrets...)
	if strings.Contains(strings.ToLower(msg), "eof") ||
		strings.Contains(strings.ToLower(msg), "timeout") ||
		strings.Contains(strings.ToLower(msg), "tls") {
		return msg + "; api.telegram.org недоступен напрямую. Задайте TG_PROXY, например socks5://127.0.0.1:1080"
	}
	return msg
}

func (b *Bot) redact(err error) string {
	secrets := append([]string{b.api.Token}, b.secrets...)
	return Redact(err, secrets...)
}

func (b *Bot) Username() string {
	return b.api.Self.UserName
}

func (b *Bot) Run(ctx context.Context) {
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		u := tgbotapi.NewUpdate(offset)
		u.Timeout = 25
		u.AllowedUpdates = []string{"message", "my_chat_member"}
		updates, err := b.api.GetUpdates(u)
		if err != nil {
			log.Printf("getUpdates: %s", b.redact(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			b.handle(upd)
		}
	}
}

func (b *Bot) handle(upd tgbotapi.Update) {
	if upd.MyChatMember != nil {
		b.handleMyChatMember(upd.MyChatMember)
		return
	}
	if upd.Message != nil {
		b.handleMembershipMessage(upd.Message)
	}
	if upd.Message == nil || !upd.Message.IsCommand() {
		return
	}
	chatID := upd.Message.Chat.ID
	switch upd.Message.Command() {
	case "start":
		b.subscribeChat(chatID, upd.Message.Chat != nil && upd.Message.Chat.IsPrivate())
	case "stop":
		if isGroupChat(upd.Message.Chat) {
			if upd.Message.From == nil || !b.canManageSubscription(upd.Message.Chat.ID, upd.Message.From.ID) {
				b.reply(chatID, "Отписать чат может только администратор.")
				return
			}
		}
		if err := b.store.Unsubscribe(chatID); err != nil {
			log.Printf("unsubscribe: %v", err)
		}
		b.reply(chatID, "Вы отписались от оповещений.")
	case "status":
		state := "не подписаны"
		if b.store.IsSubscribed(chatID) {
			state = "подписка активна"
		}
		status := "Статус: " + state + "\nРегион: Челябинская область"
		if a := b.store.ActiveDanger(); a != nil {
			status += "\nСейчас: действует " + notifier.ThreatName(*a)
			if t := alertTime(*a, b.store.LastNotifiedAt()); !t.IsZero() {
				status += " с " + t.In(time.Local).Format("02.01.2006 15:04")
			}
		} else {
			status += "\nСейчас: тревоги нет"
			if t := b.store.LastClearTime(); !t.IsZero() {
				status += "\nОтбой: " + t.In(time.Local).Format("02.01.2006 15:04")
			}
		}
		b.reply(chatID, status)
	case "last":
		last := b.store.LastAlert()
		if last == nil {
			b.reply(chatID, "Пока нет сохранённых оповещений.")
			return
		}
		b.sendAlert(chatID, *last)
	case "help":
		b.reply(chatID, "Бот следит за RSS-лентами и присылает сообщения о беспилотной/ракетной опасности по Челябинской области\nВ группе подписка включается при добавлении бота\n\n/start /stop /status /last")
	}
}

func (b *Bot) canManageSubscription(chatID, userID int64) bool {
	member, err := b.api.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	})
	if err != nil {
		log.Printf("getChatMember: %s", b.redact(err))
		return false
	}
	switch member.Status {
	case "creator", "administrator":
		return true
	default:
		return false
	}
}

func (b *Bot) handleMyChatMember(upd *tgbotapi.ChatMemberUpdated) {
	if !isGroupChat(&upd.Chat) {
		return
	}
	user := upd.NewChatMember.User
	if user == nil || user.ID != b.api.Self.ID {
		return
	}
	switch upd.NewChatMember.Status {
	case "member", "administrator", "restricted":
		b.subscribeChat(upd.Chat.ID, false)
	case "left", "kicked":
		if err := b.store.Unsubscribe(upd.Chat.ID); err != nil {
			log.Printf("unsubscribe chat: %v", err)
		}
		log.Printf("чат %d отписан: бота удалили", upd.Chat.ID)
	}
}

func (b *Bot) handleMembershipMessage(msg *tgbotapi.Message) {
	if msg.Chat == nil || !isGroupChat(msg.Chat) {
		return
	}
	for _, u := range msg.NewChatMembers {
		if u.ID == b.api.Self.ID {
			b.subscribeChat(msg.Chat.ID, false)
			return
		}
	}
	if msg.LeftChatMember != nil && msg.LeftChatMember.ID == b.api.Self.ID {
		if err := b.store.Unsubscribe(msg.Chat.ID); err != nil {
			log.Printf("unsubscribe chat: %v", err)
		}
	}
}

func (b *Bot) subscribeChat(chatID int64, private bool) {
	already := b.store.IsSubscribed(chatID)
	if err := b.store.Subscribe(chatID); err != nil {
		log.Printf("subscribe: %v", err)
	}
	if already {
		return
	}
	if private {
		b.reply(chatID, "Вы подписались на оповещения об опасности БПЛА/ракет в Челябинской области.\n\nКоманды:\n/stop — отписка\n/status — статус\n/last — последнее сообщение")
	} else {
		b.reply(chatID, "Этот чат подписан на оповещения об опасности БПЛА/ракет в Челябинской области\n/status — статус\n/last — последнее сообщение")
	}
	if a := b.store.ActiveDanger(); a != nil {
		b.sendAlert(chatID, *a)
	}
}

func isGroupChat(c *tgbotapi.Chat) bool {
	if c == nil {
		return false
	}
	switch c.Type {
	case "group", "supergroup", "channel":
		return true
	default:
		return false
	}
}

func (b *Bot) Notify(ctx context.Context, a models.Alert) error {
	text := notifier.Format(a)
	ids := b.store.ListSubscribers()
	sent := 0
	var last error
	for _, chatID := range ids {
		if ctx != nil {
			select {
			case <-ctx.Done():
				if sent == 0 {
					return ctx.Err()
				}
				return nil
			default:
			}
		}
		if err := b.sendHTML(chatID, text); err != nil {
			last = err
			continue
		}
		sent++
		time.Sleep(40 * time.Millisecond)
	}
	if len(ids) > 0 && sent == 0 && last != nil {
		return last
	}
	return nil
}

func (b *Bot) sendAlert(chatID int64, a models.Alert) {
	if err := b.sendHTML(chatID, notifier.Format(a)); err != nil && !isUnavailableChat(err) {
		log.Printf("send alert: %s", b.redact(err))
	}
}

func (b *Bot) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		if isUnavailableChat(err) {
			b.dropChat(chatID, err)
			return
		}
		log.Printf("reply: %s", b.redact(err))
	}
}

func (b *Bot) sendHTML(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	if _, err := b.api.Send(msg); err != nil {
		if isUnavailableChat(err) {
			b.dropChat(chatID, err)
			return err
		}
		log.Printf("notify %d: %s", chatID, b.redact(err))
		return err
	}
	return nil
}

func (b *Bot) dropChat(chatID int64, err error) {
	if unsubErr := b.store.Unsubscribe(chatID); unsubErr != nil {
		log.Printf("unsubscribe: %v", unsubErr)
	}
	log.Printf("чат %d отписан: %s", chatID, b.redact(err))
}

func isUnavailableChat(err error) bool {
	if err == nil {
		return false
	}
	var apiErr tgbotapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == 403 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "blocked") ||
		strings.Contains(msg, "kicked") ||
		strings.Contains(msg, "chat not found") ||
		strings.Contains(msg, "user is deactivated") ||
		strings.Contains(msg, "forbidden")
}

func alertTime(a models.Alert, notified time.Time) time.Time {
	if !a.Published.IsZero() {
		return a.Published
	}
	return notified
}
