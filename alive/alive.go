package alive

import (
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

var (
	started = time.Now()
	last    atomic.Int64
)

func Touch() {
	last.Store(time.Now().UnixNano())
}

func Serve(addr string) {
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !Healthy() {
			http.Error(w, "stale", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	go func() {
		log.Printf("healthz http://%s/healthz", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("healthz: %v", err)
		}
	}()
}

func Healthy() bool {
	t := last.Load()
	if t == 0 {
		return time.Since(started) < 2*time.Minute
	}
	return time.Since(time.Unix(0, t)) < 3*time.Minute
}
