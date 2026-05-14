package db

import (
	"database/sql"
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

	// WAL mode prevents "database is locked" errors under concurrent access
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.SetMaxOpenConns(1)

	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS operators (user_id INTEGER PRIMARY KEY)`)
	if err != nil {
		panic(err)
	}

	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		comment TEXT NOT NULL,
		subscription TEXT NOT NULL DEFAULT '',
		email_vless TEXT NOT NULL,
		email_vmess TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		panic(err)
	}

	conn.Exec(`CREATE INDEX IF NOT EXISTS idx_clients_comment ON clients(comment)`)

	// Migration for old databases lacking the subscription column
	_, err = conn.Exec(`ALTER TABLE clients ADD COLUMN subscription TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		log.Printf("ℹ️ Колонка subscription уже существует")
	}
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

func SaveClient(comment, subscription, emailVless, emailVmess string) error {
	_, err := conn.Exec(
		"INSERT INTO clients (comment, subscription, email_vless, email_vmess) VALUES (?, ?, ?, ?)",
		comment, subscription, emailVless, emailVmess,
	)
	return err
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

// GetClientEmails returns credentials and display name for a client by database ID.
func GetClientEmails(id int64) (emailVless, emailVmess, name string, err error) {
	err = conn.QueryRow(
		"SELECT email_vless, email_vmess, comment FROM clients WHERE id = ?", id,
	).Scan(&emailVless, &emailVmess, &name)
	return
}

func DeleteClient(id int64) error {
	_, err := conn.Exec("DELETE FROM clients WHERE id = ?", id)
	return err
}
