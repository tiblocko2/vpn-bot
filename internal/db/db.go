package db

import (
	"database/sql"

	"vpn-bot/internal/config"

	_ "modernc.org/sqlite"
)

// conn is the bot's small local store. Since 3X-UI v3.4.1 keeps all client data
// in its own PostgreSQL database (read via internal/xui), this SQLite file holds
// only bot-specific data that 3X-UI doesn't know about: the operators list.
var conn *sql.DB

func Init() {
	var err error
	conn, err = sql.Open("sqlite", config.Cfg.DBPath)
	if err != nil {
		panic(err)
	}

	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.SetMaxOpenConns(1)

	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS operators (user_id INTEGER PRIMARY KEY)`)
	if err != nil {
		panic(err)
	}

	// Drop the legacy bot-owned client tables: clients are now owned by 3X-UI's
	// PostgreSQL and read directly. This fully clears any old local client data.
	conn.Exec(`DROP TABLE IF EXISTS clients`)
	conn.Exec(`DROP TABLE IF EXISTS client_emails`)
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
