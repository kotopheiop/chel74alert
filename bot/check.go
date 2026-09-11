package bot

import (
	"fmt"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func CheckTelegram(token, apiEndpoint string, client *http.Client, secrets ...string) (string, error) {
	if apiEndpoint == "" {
		apiEndpoint = tgbotapi.APIEndpoint
	}
	api, err := tgbotapi.NewBotAPIWithClient(token, apiEndpoint, client)
	if err != nil {
		return "", fmt.Errorf("%s", telegramDialHint(err, append([]string{token}, secrets...)...))
	}
	if api.Self.UserName == "" {
		return "", fmt.Errorf("getMe: пустой username бота")
	}
	return api.Self.UserName, nil
}
