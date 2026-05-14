package main

import (
	"log"

	"vpn-bot/internal/bot"
	"vpn-bot/internal/config"
	"vpn-bot/internal/db"
	"vpn-bot/internal/panel"
)

func main() {
	if err := config.Load("config.json"); err != nil {
		log.Fatalf("❌ Ошибка загрузки конфига: %v\nСоздайте config.json на основе config.example.json", err)
	}

	db.Init()
	db.MigrateEmailsFromOldSchema(config.Cfg.VlessInboundID, config.Cfg.VmessInboundID)
	panel.InitHTTPClient()

	if err := panel.Login(); err != nil {
		log.Printf("⚠️ Ошибка авторизации в панели при запуске: %v", err)
	}

	b, err := bot.New()
	if err != nil {
		log.Fatalf("❌ Ошибка создания бота: %v", err)
	}
	b.Run()
}
