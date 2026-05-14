package bot

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"vpn-bot/internal/config"
	"vpn-bot/internal/db"
	"vpn-bot/internal/panel"
)

func (b *Bot) showMenu(userID int64) {
	btns := [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("➕ Добавить пользователя", "add_user")},
		{tgbotapi.NewInlineKeyboardButtonData("➖ Удалить пользователя", "del_user_list:0")},
		{tgbotapi.NewInlineKeyboardButtonData("👥 Список клиентов", "client_list:0")},
	}
	btns = append(btns, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🔄 Импорт из 3X-UI", "import_panel"),
	})
	if userID == config.Cfg.SuperUserID {
		btns = append(btns,
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("👤 Управление операторами", "ops_manage"),
			},
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("🌐 Сменить домен подписок", "change_domain"),
			},
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("⚙️ Управление inbound", "inbound_settings"),
			},
		)
	}
	msg := tgbotapi.NewMessage(userID, "🔧 Выберите действие:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(btns...)
	b.api.Send(msg)
}

func (b *Bot) showAddUser(userID int64) {
	b.userState[userID] = "waiting_name"
	msg := tgbotapi.NewMessage(userID, "📝 Введите Фамилию и Имя нового пользователя.\n\nПример: `Иванов Иван`")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "cancel")},
	)
	b.api.Send(msg)
}

// showDeleteList renders a paginated list of clients as delete buttons.
// callback_data uses numeric DB id to stay within Telegram's 64-byte limit.
func (b *Bot) showDeleteList(userID int64, page int) {
	clients, total, err := db.GetClientsPage(page)
	if err != nil {
		b.send(userID, "❌ Ошибка получения списка клиентов")
		return
	}
	if total == 0 {
		b.send(userID, "📭 Список клиентов пуст")
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(db.ClientsPerPage)))
	var buttons [][]tgbotapi.InlineKeyboardButton

	for _, c := range clients {
		label := "🗑 " + c.Comment
		if len([]rune(label)) > 50 {
			label = string([]rune(label)[:50])
		}
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("del_confirm:%d", c.ID)),
		})
	}

	if totalPages > 1 {
		var nav []tgbotapi.InlineKeyboardButton
		if page > 0 {
			nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("⬅️", fmt.Sprintf("del_user_list:%d", page-1)))
		}
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, totalPages), "noop"))
		if page < totalPages-1 {
			nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("➡️", fmt.Sprintf("del_user_list:%d", page+1)))
		}
		buttons = append(buttons, nav)
	}
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "cancel"),
	})

	msg := tgbotapi.NewMessage(userID, fmt.Sprintf("🗑 Выберите пользователя для удаления (всего: %d):", total))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	b.api.Send(msg)
}

func (b *Bot) showClientList(userID int64, page int) {
	clients, total, err := db.GetClientsPage(page)
	if err != nil {
		b.send(userID, "❌ Ошибка получения списка клиентов")
		return
	}
	if total == 0 {
		b.send(userID, "📭 Список клиентов пуст")
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(db.ClientsPerPage)))
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👥 Клиенты (всего: %d, стр. %d/%d):\n\n", total, page+1, totalPages))
	for i, c := range clients {
		sb.WriteString(fmt.Sprintf("%d. %s\n", page*db.ClientsPerPage+i+1, c.Comment))
	}

	var navBtns []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData("⬅️", fmt.Sprintf("client_list:%d", page-1)))
	}
	if page < totalPages-1 {
		navBtns = append(navBtns, tgbotapi.NewInlineKeyboardButtonData("➡️", fmt.Sprintf("client_list:%d", page+1)))
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	if len(navBtns) > 0 {
		rows = append(rows, navBtns)
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "back_to_menu"),
	})

	msg := tgbotapi.NewMessage(userID, sb.String())
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.api.Send(msg)
}

func (b *Bot) showOpsManage(userID int64) {
	operators, err := db.GetAllOperators()
	if err != nil {
		b.send(userID, "❌ Ошибка получения списка операторов")
		return
	}

	text := "👤 Операторы:\n"
	if len(operators) == 0 {
		text += "_список пуст_"
	} else {
		text += fmt.Sprintf("Всего: %d\n\n", len(operators))
		for _, id := range operators {
			text += fmt.Sprintf("• %d\n", id)
		}
	}

	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, opID := range operators {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"❌ "+strconv.FormatInt(opID, 10),
				"ops_remove:"+strconv.FormatInt(opID, 10),
			),
		})
	}
	buttons = append(buttons,
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить оператора", "ops_add"),
		},
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "back_to_menu"),
		},
	)

	msg := tgbotapi.NewMessage(userID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	b.api.Send(msg)
}

func (b *Bot) showAddOp(userID int64) {
	b.userState[userID] = "waiting_op_id"
	msg := tgbotapi.NewMessage(userID, "🆔 Введите Telegram ID нового оператора или перешлите сообщение от него.")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "cancel")},
	)
	b.api.Send(msg)
}

func (b *Bot) showInboundSettings(userID int64) {
	inbounds := config.Cfg.Inbounds
	text := "⚙️ Настроенные inbound:\n\n"
	if len(inbounds) == 0 {
		text += "_ни один не добавлен_"
	} else {
		for _, ib := range inbounds {
			text += fmt.Sprintf("• [%d] %s\n", ib.ID, ib.Label)
		}
	}

	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, ib := range inbounds {
		label := fmt.Sprintf("❌ [%d] %s", ib.ID, ib.Label)
		if len([]rune(label)) > 50 {
			label = string([]rune(label)[:50])
		}
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("ib_remove:%d", ib.ID)),
		})
	}
	buttons = append(buttons,
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить inbound", "ib_add"),
		},
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "back_to_menu"),
		},
	)

	msg := tgbotapi.NewMessage(userID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	b.api.Send(msg)
}

// showInboundPicker fetches the live inbound list from the panel and shows
// inbounds not yet configured, so the user can add one.
func (b *Bot) showInboundPicker(userID int64) {
	b.send(userID, "⏳ Загружаю список inbound из панели...")

	inbounds, err := panel.GetInboundList()
	if err != nil {
		b.send(userID, "❌ Ошибка загрузки inbound: "+err.Error())
		return
	}
	if len(inbounds) == 0 {
		b.send(userID, "❌ Inbound не найдены в панели")
		return
	}

	configured := make(map[int64]bool)
	for _, ib := range config.Cfg.Inbounds {
		configured[ib.ID] = true
	}

	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, ib := range inbounds {
		if configured[ib.ID] {
			continue
		}
		status := ""
		if !ib.Enable {
			status = " ⚫"
		}
		label := fmt.Sprintf("[%d] %s (%s)%s", ib.ID, ib.Remark, ib.Protocol, status)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("ib_add_pick:%d", ib.ID)),
		})
	}
	if len(buttons) == 0 {
		b.send(userID, "✅ Все inbound из панели уже добавлены в конфиг")
		b.showInboundSettings(userID)
		return
	}
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "inbound_settings"),
	})

	msg := tgbotapi.NewMessage(userID, "Выберите inbound для добавления:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	b.api.Send(msg)
}

// showImportSelection renders a checklist of configured inbounds for the user
// to select which ones to import from.
func (b *Bot) showImportSelection(userID int64) {
	inbounds := config.Cfg.Inbounds
	if len(inbounds) == 0 {
		b.send(userID, "❌ Нет настроенных inbound для импорта")
		return
	}

	sel := b.importSelection[userID]
	if sel == nil {
		sel = make(map[int64]bool)
		b.importSelection[userID] = sel
	}

	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, ib := range inbounds {
		mark := "⬜"
		if sel[ib.ID] {
			mark = "✅"
		}
		label := fmt.Sprintf("%s [%d] %s", mark, ib.ID, ib.Label)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("import_toggle:%d", ib.ID)),
		})
	}

	anySelected := false
	for _, v := range sel {
		if v {
			anySelected = true
			break
		}
	}

	runLabel := "⚠️ Выберите хотя бы один inbound"
	runData := "noop"
	if anySelected {
		runLabel = "▶️ Запустить импорт"
		runData = "import_run"
	}
	buttons = append(buttons,
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(runLabel, runData),
		},
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "back_to_menu"),
		},
	)

	msg := tgbotapi.NewMessage(userID, "📥 Выберите inbound для импорта клиентов:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	b.api.Send(msg)
}

func (b *Bot) showChangeDomain(userID int64) {
	b.userState[userID] = "waiting_domain"
	msg := tgbotapi.NewMessage(userID, fmt.Sprintf(
		"🌐 Текущий домен подписок:\n`%s`\n\nВведите новый базовый URL без слеша в конце.\nПример: `https://akvilon2.nemesh-vpn.ru:2096/sub`",
		config.Cfg.SubDomain,
	))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "cancel")},
	)
	b.api.Send(msg)
}
