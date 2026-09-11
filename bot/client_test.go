package bot

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestRedactHidesSecrets(t *testing.T) {
	token := "8722598397:secret-token"
	err := errors.New(`Post "https://api.telegram.org/bot` + token + `/getMe": EOF`)
	got := Redact(err, token)
	if strings.Contains(got, token) {
		t.Fatalf("токен утекает: %s", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("нет маски: %s", got)
	}
}

func TestProxyHostHidesUserinfo(t *testing.T) {
	got := ProxyHost("http://user:pass@vpn.example:42533")
	if strings.Contains(got, "user") || strings.Contains(got, "pass") {
		t.Fatalf("креды в логе: %s", got)
	}
	if got != "http://vpn.example:42533" {
		t.Fatalf("host=%q", got)
	}
}

func TestHTTPClientLongPollTimeouts(t *testing.T) {
	client, err := NewHTTPClient("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if client.Timeout != 0 {
		t.Fatalf("Timeout=%v, для getUpdates нужен 0", client.Timeout)
	}
	wrap, ok := client.Transport.(*unaryTimeoutTransport)
	if !ok {
		t.Fatal("ожидал *unaryTimeoutTransport")
	}
	if wrap.unary != UnaryTimeout {
		t.Fatalf("unary=%v", wrap.unary)
	}
	tr := innerTransport(client.Transport)
	if tr == nil {
		t.Fatal("нет внутреннего *http.Transport")
	}
	if tr.ResponseHeaderTimeout != 0 {
		t.Fatalf("ResponseHeaderTimeout=%v ломает HTTP/2 long poll", tr.ResponseHeaderTimeout)
	}
}

func TestHTTPClientSetsProxyAuth(t *testing.T) {
	client, err := NewHTTPClient("http://127.0.0.1:9", "login", "secret")
	if err != nil {
		t.Fatal(err)
	}
	tr := innerTransport(client.Transport)
	if tr == nil {
		t.Fatal("нет внутреннего *http.Transport")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.telegram.org", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.User == nil {
		t.Fatal("прокси без userinfo")
	}
	if u.User.Username() != "login" {
		t.Fatalf("user=%q", u.User.Username())
	}
	pass, _ := u.User.Password()
	if pass != "secret" {
		t.Fatalf("pass=%q", pass)
	}
}

func TestTelegramDialHintOnEOF(t *testing.T) {
	msg := telegramDialHint(errors.New("EOF"), "tok")
	if !strings.Contains(msg, "TG_PROXY") {
		t.Fatalf("нет подсказки прокси: %s", msg)
	}
}

func TestHTTPClientRejectsBadProxy(t *testing.T) {
	_, err := NewHTTPClient("://bad", "", "")
	if err == nil {
		t.Fatal("ожидал ошибку URL")
	}
}

func TestRedactHidesProxyPassword(t *testing.T) {
	err := errors.New("proxy user:hunter2 failed")
	got := Redact(err, ProxySecrets("http://user:hunter2@vpn.example:1", "user", "hunter2")...)
	if strings.Contains(got, "hunter2") || strings.Contains(got, "user") {
		t.Fatalf("креды прокси в логе: %s", got)
	}
}

func TestTelegramDialHintRedactsProxy(t *testing.T) {
	msg := telegramDialHint(errors.New("timeout hunter2"), "tok", "hunter2")
	if strings.Contains(msg, "hunter2") {
		t.Fatalf("пароль в подсказке: %s", msg)
	}
	if !strings.Contains(msg, "TG_PROXY") {
		t.Fatalf("нет подсказки прокси: %s", msg)
	}
}
