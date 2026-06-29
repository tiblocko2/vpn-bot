package main

import (
	"log"

	"vpn-bot/internal/bot"
	"vpn-bot/internal/config"
	"vpn-bot/internal/db"
	"vpn-bot/internal/panel"
	"vpn-bot/internal/xui"
)

func main() {
	if err := config.Load("config.json"); err != nil {
		log.Fatalf("❌ Ошибка загрузки конфига: %v\nСоздайте config.json на основе config.example.json", err)
	}

	log.Println("⏳ Инициализация локальной БД (операторы)...")
	db.Init()
	log.Println("⏳ Подключение к PostgreSQL 3X-UI...")
	if err := xui.Init(config.Cfg.XUIDBDSN); err != nil {
		log.Fatalf("❌ %v", err)
	}
	log.Println("⏳ Инициализация панели...")
	panel.InitHTTPClient()

	log.Printf("⏳ Подключение к Telegram API (proxy_url=%q)...", config.Cfg.ProxyURL)
	b, err := bot.New()
	if err != nil {
		log.Fatalf("❌ Ошибка создания бота: %v", err)
	}
	b.Run()
}
