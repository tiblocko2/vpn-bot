package panel

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"vpn-bot/internal/config"
	"vpn-bot/internal/db"
)

var (
	client  *http.Client
	cookies []*http.Cookie
)

func InitHTTPClient() {
	client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func Login() error {
	data := fmt.Sprintf("username=%s&password=%s", config.Cfg.PanelUsername, config.Cfg.PanelPassword)
	req, _ := http.NewRequest("POST", config.Cfg.PanelURL+"/login", strings.NewReader(data))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	cookies = resp.Cookies()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if success, ok := result["success"].(bool); !success || !ok {
		return fmt.Errorf("ошибка авторизации: %v", result["msg"])
	}

	log.Printf("✅ Авторизация в панели успешна (cookies: %d)", len(cookies))
	return nil
}

func postRequest(method string, payload interface{}) error {
	bodyBytes := []byte("{}")
	if payload != nil {
		var err error
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequest("POST", config.Cfg.PanelURL+method, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP ошибка (%s): %v", method, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		return fmt.Errorf("пустой ответ от панели (status: %d, метод: %s)", resp.StatusCode, method)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("ошибка парсинга ответа (status: %d): %v", resp.StatusCode, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return fmt.Errorf("ошибка API: %v", result["msg"])
	}
	return nil
}

func addClientToInbound(inboundID int64, email, comment, subID, uuid string) error {
	settingsJSON, _ := json.Marshal(map[string]interface{}{
		"clients": []map[string]interface{}{
			{
				"id": uuid, "alterId": 0, "email": email,
				"comment": comment, "subId": subID,
				"enable": true, "totalGB": 0, "expiryTime": 0,
				"flow": "", "tgId": "", "limitIp": 0,
			},
		},
	})
	return postRequest("/panel/api/inbounds/addClient", map[string]interface{}{
		"id":       inboundID,
		"settings": string(settingsJSON),
	})
}

func deleteClientByEmail(inboundID int64, email string) error {
	return postRequest(
		fmt.Sprintf("/panel/api/inbounds/%d/delClientByEmail/%s", inboundID, email),
		nil,
	)
}

func AddClient(name string) (string, error) {
	if err := Login(); err != nil {
		return "", fmt.Errorf("ошибка авторизации: %v", err)
	}

	subscription := normalizeName(name)
	emailVless := randomEmail()
	emailVmess := randomEmail()
	uuid := generateUUID()

	if err := addClientToInbound(config.Cfg.VlessInboundID, emailVless, name, subscription, uuid); err != nil {
		return "", fmt.Errorf("ошибка добавления в inbound %d: %v", config.Cfg.VlessInboundID, err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := addClientToInbound(config.Cfg.VmessInboundID, emailVmess, name, subscription, uuid); err != nil {
		return "", fmt.Errorf("ошибка добавления в inbound %d: %v", config.Cfg.VmessInboundID, err)
	}

	if err := db.SaveClient(name, subscription, emailVless, emailVmess); err != nil {
		return "", fmt.Errorf("ошибка сохранения в БД: %v", err)
	}

	return fmt.Sprintf("%s/%s", config.Cfg.SubDomain, subscription), nil
}

// DeleteClient removes a client from both panel inbounds and the database.
// Returns the client display name for confirmation messages.
func DeleteClient(id int64) (string, error) {
	emailVless, emailVmess, name, err := db.GetClientEmails(id)
	if err != nil {
		return "", fmt.Errorf("клиент не найден в базе")
	}

	if err := Login(); err != nil {
		return name, fmt.Errorf("ошибка авторизации: %v", err)
	}

	if err := deleteClientByEmail(config.Cfg.VlessInboundID, emailVless); err != nil {
		return name, fmt.Errorf("ошибка удаления из inbound %d: %v", config.Cfg.VlessInboundID, err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := deleteClientByEmail(config.Cfg.VmessInboundID, emailVmess); err != nil {
		return name, fmt.Errorf("ошибка удаления из inbound %d: %v", config.Cfg.VmessInboundID, err)
	}

	return name, db.DeleteClient(id)
}

// --- helpers ---

func normalizeName(name string) string {
	cyrillic := map[rune]string{
		'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d",
		'е': "e", 'ё': "yo", 'ж': "zh", 'з': "z", 'и': "i",
		'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n",
		'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
		'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch",
		'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "",
		'э': "e", 'ю': "yu", 'я': "ya",
		'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D",
		'Е': "E", 'Ё': "Yo", 'Ж': "Zh", 'З': "Z", 'И': "I",
		'Й': "Y", 'К': "K", 'Л': "L", 'М': "M", 'Н': "N",
		'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T",
		'У': "U", 'Ф': "F", 'Х': "Kh", 'Ц': "Ts", 'Ч': "Ch",
		'Ш': "Sh", 'Щ': "Shch", 'Ъ': "", 'Ы': "Y", 'Ь': "",
		'Э': "E", 'Ю': "Yu", 'Я': "Ya",
	}

	var buf strings.Builder
	for _, r := range name {
		if repl, ok := cyrillic[r]; ok {
			buf.WriteString(repl)
		} else {
			buf.WriteRune(r)
		}
	}

	s := strings.ToLower(strings.TrimSpace(buf.String()))
	s = strings.NewReplacer(" ", "_", "-", "_").Replace(s)

	var clean strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			clean.WriteRune(r)
		}
	}

	out := clean.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}

func randomEmail() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8+rand.Intn(3))
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
