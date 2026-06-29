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
	"net/http/cookiejar"
	"regexp"
	"strings"
	"time"

	"vpn-bot/internal/config"
)

var client *http.Client

func InitHTTPClient() {
	jar, _ := cookiejar.New(nil)
	client = &http.Client{
		Timeout: 90 * time.Second,
		Jar:     jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// ensureAuth authenticates if no Bearer token is configured.
// With a Bearer token the HTTP client sends Authorization header on every request
// and no session/CSRF management is needed.
func ensureAuth() error {
	if config.Cfg.PanelAPIToken != "" {
		return nil // Bearer token — no login needed
	}
	return Login()
}

// addAuthHeaders sets the required auth and request-type headers.
func addAuthHeaders(req *http.Request) {
	if config.Cfg.PanelAPIToken != "" {
		req.Header.Set("Authorization", "Bearer "+config.Cfg.PanelAPIToken)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
}

// fetchCSRFToken loads the login page and extracts the CSRF meta tag value.
// 3X-UI ≥ v3.1.0 requires the token on every POST when using cookie auth.
func fetchCSRFToken() (string, error) {
	req, err := http.NewRequest("GET", config.Cfg.PanelURL+"/login", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// <meta name="csrf-token" content="TOKEN">
	re := regexp.MustCompile(`<meta\s+name=["']csrf-token["']\s+content=["']([^"']+)["']`)
	m := re.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("CSRF токен не найден на странице логина")
	}
	return string(m[1]), nil
}

// Login performs cookie-based login with automatic CSRF token extraction.
// The extracted token is sent in the X-CSRF-Token header as required by 3X-UI v3.1.0+.
func Login() error {
	// Flush old cookies so we get a fresh session each call.
	jar, _ := cookiejar.New(nil)
	client.Jar = jar

	// Step 1: fetch login page to get CSRF token (ignore errors — older panels may not need it).
	csrfToken, err := fetchCSRFToken()
	if err != nil {
		log.Printf("ℹ️ CSRF токен не получен (%v), продолжаем без него", err)
	}

	// Step 2: POST credentials.
	data := fmt.Sprintf("username=%s&password=%s", config.Cfg.PanelUsername, config.Cfg.PanelPassword)
	req, _ := http.NewRequest("POST", config.Cfg.PanelURL+"/login", strings.NewReader(data))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", config.Cfg.PanelURL+"/")
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		return fmt.Errorf("пустой ответ от панели (HTTP %d) — проверьте Web Base Path и доступность панели", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("ответ не является JSON (HTTP %d): %.300s", resp.StatusCode, string(body))
	}
	if success, ok := result["success"].(bool); !success || !ok {
		return fmt.Errorf("ошибка авторизации: %v", result["msg"])
	}

	log.Printf("✅ Авторизация в панели успешна")
	return nil
}

func postRequest(path string, payload interface{}) error {
	bodyBytes := []byte("{}")
	if payload != nil {
		var err error
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequest("POST", config.Cfg.PanelURL+path, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	addAuthHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP ошибка (%s): %v", path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		return fmt.Errorf("пустой ответ от панели (status: %d, метод: %s)", resp.StatusCode, path)
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

// addClientToPanel calls POST /panel/api/clients/add (3X-UI v3.4.1).
// One email/subId/UUID identity is shared across every specified inbound.
func addClientToPanel(inboundIDs []int64, email, comment, subID, uuid string) error {
	return postRequest("/panel/api/clients/add", map[string]interface{}{
		"client": map[string]interface{}{
			"id":         uuid,
			"email":      email,
			"comment":    comment,
			"subId":      subID,
			"enable":     true,
			"totalGB":    0,
			"expiryTime": 0,
			"flow":       "",
			"tgId":       0,
			"limitIp":    0,
			"reset":      0,
		},
		"inboundIds": inboundIDs,
	})
}

// AddClient creates a brand-new client and registers it in every configured inbound.
func AddClient(name string) (string, error) {
	if err := ensureAuth(); err != nil {
		return "", fmt.Errorf("ошибка авторизации: %v", err)
	}

	inbounds := config.Cfg.Inbounds
	if len(inbounds) == 0 {
		return "", fmt.Errorf("не настроены inbound в конфиге")
	}

	subscription := normalizeName(name)
	uuid := generateUUID()
	email := randomEmail() // one email shared across all inbounds

	inboundIDs := make([]int64, len(inbounds))
	for i, ib := range inbounds {
		inboundIDs[i] = ib.ID
	}

	if err := addClientToPanel(inboundIDs, email, name, subscription, uuid); err != nil {
		return "", fmt.Errorf("ошибка добавления клиента: %v", err)
	}

	return fmt.Sprintf("%s/%s", config.Cfg.SubDomain, subscription), nil
}

// DeleteClient removes a client from every inbound (and from the 3X-UI database)
// by its email — the client's stable identity in v3.4.1.
func DeleteClient(email string) error {
	if err := ensureAuth(); err != nil {
		return fmt.Errorf("ошибка авторизации: %v", err)
	}
	if err := postRequest("/panel/api/clients/del/"+email, nil); err != nil {
		return fmt.Errorf("ошибка удаления клиента: %v", err)
	}
	return nil
}

// AttachClientToInbound attaches an existing client to additional inbounds,
// reusing its identity: POST /panel/api/clients/:email/attach (3X-UI v3.4.1).
func AttachClientToInbound(email string, inboundIDs []int64) error {
	if err := ensureAuth(); err != nil {
		return fmt.Errorf("ошибка авторизации: %v", err)
	}
	return postRequest(
		"/panel/api/clients/"+email+"/attach",
		map[string]interface{}{"inboundIds": inboundIDs},
	)
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
