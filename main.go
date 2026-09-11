package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chel74alert/alive"
	"chel74alert/bot"
	"chel74alert/config"
	"chel74alert/models"
	"chel74alert/sources"
	"chel74alert/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("оповещения74 ")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("конфиг: %v", err)
	}
	if cfg.Token == "" {
		log.Fatal("задайте TG_BOT_TOKEN (токен от @BotFather)")
	}

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("хранилище: %v", err)
	}
	defer st.Close()

	client, err := bot.NewHTTPClient(cfg.Proxy, cfg.ProxyUser, cfg.ProxyPassword)
	if err != nil {
		log.Fatalf("прокси: %v", err)
	}
	secrets := bot.ProxySecrets(cfg.Proxy, cfg.ProxyUser, cfg.ProxyPassword)
	if cfg.Proxy != "" {
		log.Printf("Telegram через прокси %s", bot.ProxyHost(cfg.Proxy))
	}

	if os.Getenv("SKIP_PREFLIGHT") != "1" {
		preCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		log.Println("preflight: проверка RSS...")
		if err := sources.CheckFeeds(preCtx, cfg); err != nil {
			cancel()
			log.Fatalf("preflight RSS: %v", err)
		}
		log.Println("preflight: проверка Telegram getMe...")
		name, err := bot.CheckTelegram(cfg.Token, "", client, secrets...)
		cancel()
		if err != nil {
			log.Fatalf("preflight getMe: %v", err)
		}
		log.Printf("preflight ok, бот @%s", name)
	}

	tg, err := bot.New(cfg.Token, st, client, secrets...)
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}
	log.Printf("бот @%s запущен, интервал опроса %s", tg.Username(), cfg.PollInterval)

	alive.Serve(cfg.HealthAddr)
	alive.Touch()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	alerts := make(chan models.Alert, 32)

	for _, a := range st.PendingAlerts() {
		log.Printf("повтор недоставленного [%s] %s", a.Kind, a.Title)
		if err := tg.Notify(ctx, a); err != nil {
			log.Printf("notify: %s", bot.Redact(err, append([]string{cfg.Token}, secrets...)...))
			continue
		}
		if err := st.AckPending(a.ID); err != nil {
			log.Printf("store: %v", err)
		}
		alive.Touch()
	}

	go tg.Run(ctx)
	go sources.Start(ctx, cfg, st, alerts)

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("остановка...")
			drainCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			drainAlerts(drainCtx, tg, st, alerts)
			cancel()
			return
		case <-heartbeat.C:
			alive.Touch()
		case a := <-alerts:
			log.Printf("оповещение [%s] %s", a.Kind, a.Title)
			if err := tg.Notify(ctx, a); err != nil {
				log.Printf("notify: %s", bot.Redact(err, append([]string{cfg.Token}, secrets...)...))
				continue
			}
			if err := st.AckPending(a.ID); err != nil {
				log.Printf("store: %v", err)
			}
			alive.Touch()
		}
	}
}

func drainAlerts(ctx context.Context, tg *bot.Bot, st *store.Store, alerts <-chan models.Alert) {
	timer := time.NewTimer(400 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case a := <-alerts:
			_ = tg.Notify(ctx, a)
			_ = st.AckPending(a.ID)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(300 * time.Millisecond)
		case <-timer.C:
			return
		}
	}
}
