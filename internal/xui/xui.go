// Package xui reads client and inbound data directly from the 3X-UI PostgreSQL
// database (v3.4.1+). In v3.4.1 clients became first-class rows in the `clients`
// and `client_inbounds` tables instead of being embedded in each inbound's
// settings JSON, so the bot treats this database as the source of truth for
// listing/searching/inspecting clients. Mutations still go through the panel API
// (see internal/panel), which writes to this same database.
package xui

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// ClientsPerPage is the page size used by GetClientsPage.
const ClientsPerPage = 10

var conn *sql.DB

// Client is the bot's view of a 3X-UI client row.
type Client struct {
	ID      int64
	Email   string
	SubID   string
	Comment string
	UUID    string
	Enable  bool
}

// Inbound is a minimal view of a 3X-UI inbound row.
type Inbound struct {
	ID       int64
	Remark   string
	Protocol string
	Enable   bool
}

// Init opens the PostgreSQL connection to the 3X-UI database and verifies it.
func Init(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("xui_db_dsn не задан в конфиге")
	}
	var err error
	conn, err = sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("ошибка открытия PostgreSQL: %v", err)
	}
	conn.SetMaxOpenConns(5)
	conn.SetMaxIdleConns(2)
	conn.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("ошибка подключения к PostgreSQL 3X-UI: %v", err)
	}
	return nil
}

// GetClientsPage returns one page of clients (optionally filtered by search) and
// the total number of rows matching the same filter. search matches case-insensitively
// against comment, email, and subId.
func GetClientsPage(search string, page, pageSize int) (clients []Client, total int, err error) {
	where := ""
	args := []interface{}{}
	if search != "" {
		where = "WHERE comment ILIKE '%' || $1 || '%' OR email ILIKE '%' || $1 || '%' OR sub_id ILIKE '%' || $1 || '%'"
		args = append(args, search)
	}

	countQuery := "SELECT COUNT(*) FROM clients " + where
	if err = conn.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ошибка подсчёта клиентов: %v", err)
	}

	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	listQuery := fmt.Sprintf(
		"SELECT id, email, sub_id, comment, uuid, enable FROM clients %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, limitPos, offsetPos,
	)
	args = append(args, pageSize, page*pageSize)

	rows, err := conn.Query(listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка выборки клиентов: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Client
		if err = rows.Scan(&c.ID, &c.Email, &c.SubID, &c.Comment, &c.UUID, &c.Enable); err != nil {
			return nil, 0, err
		}
		clients = append(clients, c)
	}
	return clients, total, rows.Err()
}

// GetClientByID returns a single client by its primary key.
func GetClientByID(id int64) (Client, error) {
	var c Client
	err := conn.QueryRow(
		"SELECT id, email, sub_id, comment, uuid, enable FROM clients WHERE id = $1", id,
	).Scan(&c.ID, &c.Email, &c.SubID, &c.Comment, &c.UUID, &c.Enable)
	if err == sql.ErrNoRows {
		return c, fmt.Errorf("клиент не найден")
	}
	return c, err
}

// GetClientInbounds returns the inbounds a client is attached to.
func GetClientInbounds(clientID int64) ([]Inbound, error) {
	rows, err := conn.Query(`
		SELECT i.id, i.remark, i.protocol, i.enable
		FROM client_inbounds ci
		JOIN inbounds i ON i.id = ci.inbound_id
		WHERE ci.client_id = $1
		ORDER BY i.id`, clientID)
	if err != nil {
		return nil, fmt.Errorf("ошибка выборки inbound клиента: %v", err)
	}
	defer rows.Close()
	var out []Inbound
	for rows.Next() {
		var ib Inbound
		if err := rows.Scan(&ib.ID, &ib.Remark, &ib.Protocol, &ib.Enable); err != nil {
			return nil, err
		}
		out = append(out, ib)
	}
	return out, rows.Err()
}

// ClientExistsByComment reports whether a client with the given display name
// (comment) already exists.
func ClientExistsByComment(comment string) (bool, error) {
	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM clients WHERE comment = $1", comment).Scan(&count)
	return count > 0, err
}

// GetInboundList returns all inbounds in the panel.
func GetInboundList() ([]Inbound, error) {
	rows, err := conn.Query("SELECT id, remark, protocol, enable FROM inbounds ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("ошибка выборки inbound: %v", err)
	}
	defer rows.Close()
	var out []Inbound
	for rows.Next() {
		var ib Inbound
		if err := rows.Scan(&ib.ID, &ib.Remark, &ib.Protocol, &ib.Enable); err != nil {
			return nil, err
		}
		out = append(out, ib)
	}
	return out, rows.Err()
}
