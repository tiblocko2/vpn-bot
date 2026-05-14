package db

import (
	"database/sql"
	"fmt"
	"log"

	"vpn-bot/internal/config"

	_ "modernc.org/sqlite"
)

var conn *sql.DB

const ClientsPerPage = 10

type ClientRecord struct {
	ID      int64
	Comment string
}

func Init() {
	var err error
	conn, err = sql.Open("sqlite", config.Cfg.DBPath)
	if err != nil {
		panic(err)
	}

	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.Exec("PRAGMA foreign_keys=ON")
	conn.SetMaxOpenConns(1)

	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS operators (user_id INTEGER PRIMARY KEY)`)
	if err != nil {
		panic(err)
	}

	// Legacy columns email_vless/email_vmess kept for backward compat (always '').
	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		comment TEXT NOT NULL,
		subscription TEXT NOT NULL DEFAULT '',
		email_vless TEXT NOT NULL DEFAULT '',
		email_vmess TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		panic(err)
	}

	conn.Exec(`CREATE INDEX IF NOT EXISTS idx_clients_comment ON clients(comment)`)

	// client_emails is the authoritative source for inbound→email mapping.
	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS client_emails (
		client_id INTEGER NOT NULL,
		inbound_id INTEGER NOT NULL,
		email TEXT NOT NULL,
		PRIMARY KEY (client_id, inbound_id),
		FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
	)`)
	if err != nil {
		panic(err)
	}

	// One-time migration: add subscription column to old databases
	if _, err = conn.Exec(`ALTER TABLE clients ADD COLUMN subscription TEXT NOT NULL DEFAULT ''`); err != nil {
		log.Printf("ℹ️ Колонка subscription уже существует")
	}
}

// MigrateEmailsFromOldSchema copies email_vless/email_vmess data into
// client_emails using the two legacy inbound IDs. This runs once on first
// start after upgrading from v1.0/v1.1.
func MigrateEmailsFromOldSchema(vlessID, vmessID int64) {
	if vlessID == 0 && vmessID == 0 {
		return
	}
	var existing int
	conn.QueryRow("SELECT COUNT(*) FROM client_emails").Scan(&existing)
	if existing > 0 {
		return // already migrated
	}
	var count int
	conn.QueryRow("SELECT COUNT(*) FROM clients WHERE email_vless != '' OR email_vmess != ''").Scan(&count)
	if count == 0 {
		return
	}

	rows, err := conn.Query("SELECT id, email_vless, email_vmess FROM clients WHERE email_vless != '' OR email_vmess != ''")
	if err != nil {
		log.Printf("⚠️ Ошибка миграции emails: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var ev, em string
		rows.Scan(&id, &ev, &em)
		if vlessID != 0 && ev != "" {
			conn.Exec("INSERT OR IGNORE INTO client_emails VALUES (?, ?, ?)", id, vlessID, ev)
		}
		if vmessID != 0 && em != "" {
			conn.Exec("INSERT OR IGNORE INTO client_emails VALUES (?, ?, ?)", id, vmessID, em)
		}
	}
	log.Printf("✅ Мигрировано %d клиентов в client_emails", count)
}

func IsOperator(userID int64) bool {
	if userID == config.Cfg.SuperUserID {
		return true
	}
	var id int64
	return conn.QueryRow("SELECT user_id FROM operators WHERE user_id = ?", userID).Scan(&id) == nil
}

func AddOperator(userID int64) error {
	_, err := conn.Exec("INSERT OR IGNORE INTO operators VALUES (?)", userID)
	return err
}

func RemoveOperator(userID int64) error {
	_, err := conn.Exec("DELETE FROM operators WHERE user_id = ?", userID)
	return err
}

func GetAllOperators() ([]int64, error) {
	rows, err := conn.Query("SELECT user_id FROM operators")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ops = append(ops, id)
	}
	return ops, nil
}

func ClientExists(comment string) bool {
	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM clients WHERE comment = ?", comment).Scan(&count)
	return err == nil && count > 0
}

func ClientExistsBySubscription(subscription string) bool {
	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM clients WHERE subscription = ?", subscription).Scan(&count)
	return err == nil && count > 0
}

// SaveClient creates a client record and stores one email per inbound.
func SaveClient(comment, subscription string, emails map[int64]string) error {
	result, err := conn.Exec(
		"INSERT INTO clients (comment, subscription, email_vless, email_vmess) VALUES (?, ?, '', '')",
		comment, subscription,
	)
	if err != nil {
		return err
	}
	clientID, _ := result.LastInsertId()
	for inboundID, email := range emails {
		if _, err := conn.Exec(
			"INSERT OR IGNORE INTO client_emails (client_id, inbound_id, email) VALUES (?, ?, ?)",
			clientID, inboundID, email,
		); err != nil {
			return fmt.Errorf("ошибка записи email для inbound %d: %v", inboundID, err)
		}
	}
	return nil
}

// GetClientEmails returns all inbound→email pairs for a client and its display name.
func GetClientEmails(clientID int64) (emails map[int64]string, name string, err error) {
	err = conn.QueryRow("SELECT comment FROM clients WHERE id = ?", clientID).Scan(&name)
	if err != nil {
		return nil, "", fmt.Errorf("клиент не найден")
	}
	rows, err := conn.Query("SELECT inbound_id, email FROM client_emails WHERE client_id = ?", clientID)
	if err != nil {
		return nil, name, err
	}
	defer rows.Close()
	emails = make(map[int64]string)
	for rows.Next() {
		var ibID int64
		var email string
		if err = rows.Scan(&ibID, &email); err != nil {
			return
		}
		emails[ibID] = email
	}
	return
}

// GetClientsPage returns one page of clients and total count.
// Using LIMIT/OFFSET prevents sending hundreds of buttons to Telegram at once.
func GetClientsPage(page int) (clients []ClientRecord, total int, err error) {
	err = conn.QueryRow("SELECT COUNT(*) FROM clients").Scan(&total)
	if err != nil {
		return
	}
	rows, err := conn.Query(
		"SELECT id, comment FROM clients ORDER BY created_at DESC LIMIT ? OFFSET ?",
		ClientsPerPage, page*ClientsPerPage,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c ClientRecord
		if err = rows.Scan(&c.ID, &c.Comment); err != nil {
			return
		}
		clients = append(clients, c)
	}
	return
}

func DeleteClient(id int64) error {
	_, err := conn.Exec("DELETE FROM clients WHERE id = ?", id)
	return err
}
