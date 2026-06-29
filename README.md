# VPN Bot

Telegram-бот для управления клиентами 3X-UI (VLESS + VMess).

Требуется **3X-UI v3.4.1+** с базой данных в **PostgreSQL**. Список, поиск и детали клиентов
читаются ботом напрямую из базы `xui` (источник правды); создание/удаление/привязка идут через
API панели `/panel/api/clients/*`. Локально (SQLite) бот хранит только список операторов.

## Установка на сервере Ubuntu/Debian

```bash
git clone https://github.com/tiblocko2/vpn-bot.git
cd vpn-bot
sudo bash install.sh
```

Скрипт интерактивно спросит все необходимые параметры и установит бота как systemd-сервис.  
Бинарный файл автоматически загружается из последнего GitHub Release.

### Управление сервисом

```bash
systemctl status vpn-bot      # статус
journalctl -u vpn-bot -f      # логи в реальном времени
systemctl restart vpn-bot     # перезапуск
```

## Обновление

```bash
cd /opt/vpn-bot
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL https://github.com/tiblocko2/vpn-bot/releases/latest/download/vpn-bot-linux-$ARCH -o vpn-bot
chmod +x vpn-bot
systemctl restart vpn-bot
```

## Конфигурация

Файл: `/opt/vpn-bot/config.json` (шаблон: [`config.example.json`](config.example.json))

| Параметр | Описание |
|---|---|
| `bot_token` | Токен Telegram-бота (@BotFather) |
| `super_user_id` | Telegram ID суперпользователя |
| `panel_url` | URL панели 3X-UI с web base path, без слеша в конце |
| `panel_username` / `panel_password` | Логин и пароль от панели |
| `panel_api_token` | API Token панели (опц., вместо логина/пароля) |
| `xui_db_dsn` | DSN PostgreSQL базы 3X-UI (`postgres://user:pass@host:5432/xui?sslmode=disable`) |
| `sub_domain` | Базовый URL для ссылок на подписки |
| `inbounds` | Список inbound (`id` + `label`), куда добавлять новых клиентов |
| `proxy_url` | Прокси для Telegram API (необязательно) |
| `db_path` | Путь к локальной SQLite-базе (только операторы) |

Домен подписок (`sub_domain`) можно менять **прямо через бота** — кнопка "🌐 Сменить домен подписок" в меню суперпользователя.

В списке клиентов доступны **поиск** (по имени / email / subId) и **переход на страницу по номеру**
(клик по индикатору `стр. N/M` → ввод номера).

## Структура проекта

```
vpn-bot/
├── cmd/bot/           # точка входа (main.go)
├── internal/
│   ├── config/        # загрузка и сохранение конфига
│   ├── db/            # локальный SQLite (операторы)
│   ├── xui/           # чтение клиентов/inbound из PostgreSQL 3X-UI
│   ├── panel/         # API-клиент 3X-UI (создание/удаление/привязка)
│   └── bot/           # Telegram-бот (bot, views, callbacks)
├── .github/workflows/ # CI/CD: сборка релизов при пуше тега
├── config.example.json
└── install.sh
```

## Сборка из исходников

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o vpn-bot ./cmd/bot/

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o vpn-bot ./cmd/bot/
```
