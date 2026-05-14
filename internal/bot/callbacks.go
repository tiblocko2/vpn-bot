package bot

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"vpn-bot/internal/config"
	"vpn-bot/internal/db"
	"vpn-bot/internal/panel"
)

func (b *Bot) handleCallback(update *tgbotapi.Update) {
	cb := update.CallbackQuery
	userID := cb.From.ID

	if !db.IsOperator(userID) {
		b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		b.send(userID, "❌ У вас нет доступа к боту")
		return
	}

	ack := func(text string) { b.api.Request(tgbotapi.NewCallback(cb.ID, text)) }
	data := cb.Data

	switch {
	case data == "add_user":
		ack("")
		b.showAddUser(userID)

	case strings.HasPrefix(data, "del_user_list:"):
		ack("")
		page, _ := strconv.Atoi(strings.TrimPrefix(data, "del_user_list:"))
		b.showDeleteList(userID, page)

	case strings.HasPrefix(data, "client_list:"):
		ack("")
		page, _ := strconv.Atoi(strings.TrimPrefix(data, "client_list:"))
		b.showClientList(userID, page)

	case strings.HasPrefix(data, "del_confirm:"):
		ack("")
		id, _ := strconv.ParseInt(strings.TrimPrefix(data, "del_confirm:"), 10, 64)
		name, err := panel.DeleteClient(id)
		if err != nil {
			b.send(userID, "❌ Ошибка: "+err.Error())
		} else {
			b.send(userID, fmt.Sprintf("✅ Пользователь '%s' успешно удалён", name))
		}

	case data == "ops_manage":
		ack("")
		b.showOpsManage(userID)

	case data == "ops_add":
		ack("")
		b.showAddOp(userID)

	case strings.HasPrefix(data, "ops_remove:"):
		ack("")
		opID, _ := strconv.ParseInt(strings.TrimPrefix(data, "ops_remove:"), 10, 64)
		if err := db.RemoveOperator(opID); err != nil {
			b.send(userID, "❌ Ошибка удаления оператора")
		} else {
			b.send(userID, fmt.Sprintf("✅ Оператор %d удалён", opID))
		}

	case data == "import_panel":
		ack("")
		b.send(userID, "⏳ Импортирую клиентов из 3X-UI...")
		res, err := panel.ImportClientsFromPanel()
		if err != nil {
			b.send(userID, "❌ Ошибка импорта: "+err.Error())
		} else {
			b.send(userID, fmt.Sprintf(
				"✅ Импорт завершён:\n• Добавлено: %d\n• Уже существуют: %d\n• Только в одном inbound: %d",
				res.Imported, res.Skipped, res.Orphaned,
			))
		}

	case data == "change_domain":
		if userID != config.Cfg.SuperUserID {
			ack("❌ Нет доступа")
			return
		}
		ack("")
		b.showChangeDomain(userID)

	case data == "cancel":
		ack("❌ Отменено")
		delete(b.userState, userID)
		b.showMenu(userID)

	case data == "back_to_menu":
		ack("")
		b.showMenu(userID)

	case data == "noop":
		ack("")
	}
}
