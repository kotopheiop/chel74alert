package alive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthyDuringStartup(t *testing.T) {
	started = time.Now()
	last.Store(0)
	if !Healthy() {
		t.Fatal("сразу после старта healthz должен быть ок")
	}
}

func TestHealthyStaleAfterTouchWindow(t *testing.T) {
	Touch()
	last.Store(time.Now().Add(-4 * time.Minute).UnixNano())
	if Healthy() {
		t.Fatal("после 3 минут без Touch healthz должен быть stale")
	}
	Touch()
	if !Healthy() {
		t.Fatal("после Touch healthz снова ок")
	}
}

func TestHealthzHandler(t *testing.T) {
	Touch()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !Healthy() {
			http.Error(w, "stale", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
}
