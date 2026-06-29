#!/bin/bash
# VPN Bot — интерактивный установщик для Ubuntu/Debian
# Использование: sudo bash install.sh
set -e

INSTALL_DIR="/opt/vpn-bot"
SERVICE_NAME="vpn-bot"
REPO="tiblocko2/vpn-bot"
BINARY="vpn-bot"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${GREEN}$1${NC}"; }
warn()    { echo -e "${YELLOW}$1${NC}"; }
error()   { echo -e "${RED}$1${NC}"; exit 1; }
section() { echo -e "\n${CYAN}=== $1 ===${NC}\n"; }

[ "$EUID" -ne 0 ] && error "Запустите скрипт с правами root: sudo bash install.sh"

# --- Determine architecture and download binary ---

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  BIN_ARCH="amd64" ;;
    aarch64) BIN_ARCH="arm64" ;;
    *)       error "Неподдерживаемая архитектура: $ARCH" ;;
esac

mkdir -p "$INSTALL_DIR"

if [ -f "./$BINARY" ]; then
    warn "⚠️  Найден локальный бинарный файл — использую его."
    cp "./$BINARY" "$INSTALL_DIR/$BINARY"
else
    DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/vpn-bot-linux-$BIN_ARCH"
    info "📥 Загрузка vpn-bot (linux/$BIN_ARCH)..."
    curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/$BINARY" \
        || error "Не удалось загрузить бинарный файл. Проверьте наличие релиза на GitHub."
fi
chmod +x "$INSTALL_DIR/$BINARY"

# --- Interactive configuration ---

section "Telegram"

read -p "Bot Token: " BOT_TOKEN
[ -z "$BOT_TOKEN" ] && error "Bot Token не может быть пустым"

read -p "Ваш Telegram ID (SuperUser): " SUPER_USER_ID
[[ ! "$SUPER_USER_ID" =~ ^[0-9]+$ ]] && error "Telegram ID должен быть числом"

section "Панель 3X-UI"

read -p "URL панели (например: https://srv.example.com:808): " PANEL_URL
[ -z "$PANEL_URL" ] && error "URL панели не может быть пустым"
PANEL_URL="${PANEL_URL%/}"  # убираем trailing slash

read -p "Логин: " PANEL_USERNAME
[ -z "$PANEL_USERNAME" ] && error "Логин не может быть пустым"

read -s -p "Пароль: " PANEL_PASSWORD
echo ""
[ -z "$PANEL_PASSWORD" ] && error "Пароль не может быть пустым"

echo ""
info "💡 API Token (рекомендуется для 3X-UI v3.1.0+)"
info "   Получить: 3X-UI → Settings → Security → API Tokens → Create"
read -p "   API Token панели [Enter — пропустить, авторизация по паролю]: " PANEL_API_TOKEN

section "База данных 3X-UI (PostgreSQL)"
info "3X-UI v3.4.1+ хранит клиентов в PostgreSQL. Бот читает их напрямую."
info "DSN можно увидеть в логе панели при старте."
read -p "PostgreSQL DSN [postgres://user:pass@127.0.0.1:5432/xui?sslmode=disable]: " XUI_DB_DSN
[ -z "$XUI_DB_DSN" ] && error "PostgreSQL DSN не может быть пустым"

section "Inbound'ы"
info "Введите ID и название каждого inbound'а."
info "Нажмите Enter без ID — чтобы закончить."

INBOUNDS_JSON=""
IB_NUM=0

while true; do
    echo ""
    read -p "  ID inbound #$((IB_NUM + 1)): " IB_ID
    if [ -z "$IB_ID" ]; then
        [ "$IB_NUM" -eq 0 ] && warn "  Добавьте хотя бы один inbound." && continue
        break
    fi
    [[ ! "$IB_ID" =~ ^[0-9]+$ ]] && warn "  ID должен быть числом." && continue

    read -p "  Название (например: VLESS, VMess): " IB_LABEL
    [ -z "$IB_LABEL" ] && IB_LABEL="Inbound$((IB_NUM + 1))"

    [ -n "$INBOUNDS_JSON" ] && INBOUNDS_JSON="$INBOUNDS_JSON,"
    INBOUNDS_JSON="${INBOUNDS_JSON}
    {\"id\": $IB_ID, \"label\": \"$IB_LABEL\"}"
    IB_NUM=$((IB_NUM + 1))
    info "  ✅ Добавлен: $IB_LABEL (ID=$IB_ID)"
done

section "Подписки"

read -p "Базовый URL подписок (например: https://srv.example.com:2096/sub): " SUB_DOMAIN
[ -z "$SUB_DOMAIN" ] && error "Домен подписок не может быть пустым"
SUB_DOMAIN="${SUB_DOMAIN%/}"

read -p "Прокси URL [Enter — не использовать]: " PROXY_URL

DB_PATH="$INSTALL_DIR/db.sqlite"

# --- Write config.json ---

info "\n📝 Создание $INSTALL_DIR/config.json..."

cat > "$INSTALL_DIR/config.json" <<CONFIGEOF
{
  "bot_token": "$BOT_TOKEN",
  "super_user_id": $SUPER_USER_ID,
  "panel_url": "$PANEL_URL",
  "panel_username": "$PANEL_USERNAME",
  "panel_password": "$PANEL_PASSWORD",
  "panel_api_token": "$PANEL_API_TOKEN",
  "xui_db_dsn": "$XUI_DB_DSN",
  "sub_domain": "$SUB_DOMAIN",
  "proxy_url": "$PROXY_URL",
  "inbounds": [$INBOUNDS_JSON
  ],
  "db_path": "$DB_PATH"
}
CONFIGEOF
chmod 600 "$INSTALL_DIR/config.json"

# Migrate existing database if present
if [ -f "./db.sqlite" ] && [ ! -f "$DB_PATH" ]; then
    warn "📦 Найдена существующая db.sqlite — копирую в $INSTALL_DIR/"
    cp "./db.sqlite" "$DB_PATH"
fi

# --- Systemd service ---

info "⚙️  Создание systemd-сервиса $SERVICE_NAME..."

cat > "/etc/systemd/system/$SERVICE_NAME.service" <<SVCEOF
[Unit]
Description=VPN Telegram Bot
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/$BINARY
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SVCEOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

echo ""
info "✅ Установка завершена!"
echo ""
echo "  Статус:  systemctl status $SERVICE_NAME"
echo "  Логи:    journalctl -u $SERVICE_NAME -f"
echo "  Конфиг:  $INSTALL_DIR/config.json"
echo ""
info "🔄 Обновление бинарника в будущем:"
echo ""
echo "  cd /opt/vpn-bot"
echo "  ARCH=\$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
echo "  curl -fsSL https://github.com/$REPO/releases/latest/download/vpn-bot-linux-\$ARCH -o vpn-bot"
echo "  chmod +x vpn-bot"
echo "  systemctl restart vpn-bot"
echo ""
