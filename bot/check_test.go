package bot

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckTelegramGetMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getMe") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"t","username":"chel74_alert_bot"}}`)
	}))
	t.Cleanup(srv.Close)

	name, err := CheckTelegram("TESTTOKEN", srv.URL+"/bot%s/%s", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if name != "chel74_alert_bot" {
		t.Fatalf("username=%q", name)
	}
}

func TestCheckTelegramFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	if _, err := CheckTelegram("bad", srv.URL+"/bot%s/%s", srv.Client()); err == nil {
		t.Fatal("ожидал ошибку getMe")
	}
}
