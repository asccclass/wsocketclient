package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// ──────────────────────────────────────────────
// 設定結構體（從 .env 載入）
// ──────────────────────────────────────────────

type Config struct {
	UserID       string // BOT_USER_ID
	DisplayName  string // BOT_DISPLAY_NAME
	Channel      string // BOT_CHANNEL
	OpenclawBin  string // OPENCLAW_BIN
	WebsocketURL string // 由 WEBSOCKET_BASE_URL + BOT_USER_ID 組合
}

// loadConfig 讀取 .env 檔並回傳 Config，缺少必要欄位時直接 fatal
func loadConfig() Config {
	// 若 .env 不存在則略過（允許直接用環境變數）
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  找不到 .env 檔，改從環境變數讀取")
	}

	mustGet := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			log.Fatalf("❌ 必要環境變數 %q 未設定，請檢查 .env 檔", key)
		}
		return v
	}

	userID := mustGet("BOT_USER_ID")
	baseURL := mustGet("WEBSOCKET_BASE_URL") // e.g. wss://jarvis.justdrink.com.tw/webhook

	return Config{
		UserID:       userID,
		DisplayName:  mustGet("BOT_DISPLAY_NAME"),
		Channel:      mustGet("BOT_CHANNEL"),
		OpenclawBin:  mustGet("OPENCLAW_BIN"),
		WebsocketURL: baseURL + "?user=" + userID,
	}
}

// 全域設定（main 初始化後唯讀）
var cfg Config

// ──────────────────────────────────────────────
// 訊息結構體
// ──────────────────────────────────────────────

// IncomingMessage 代表從 WebSocket 收到的訊息格式
type IncomingMessage struct {
	Channel     string `json:"channel"`
	Message     string `json:"message"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Data        string `json:"data"`
	ReplyTo     string `json:"reply_to"`
	Type        string `json:"type"`
}

// OutgoingMessage 代表要送回 WebSocket 的回應格式
type OutgoingMessage struct {
	Channel     string `json:"channel"`
	Message     string `json:"message"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Data        string `json:"data"`
	ReplyTo     string `json:"reply_to"`
	Type        string `json:"type"`
}

// ──────────────────────────────────────────────
// OpenClaw CLI 結構體（用於解析 JSON 輸出）
// ──────────────────────────────────────────────

type SessionsOutput struct {
	Sessions []struct {
		SessionID string `json:"sessionId"`
		Kind      string `json:"kind"`
	} `json:"sessions"`
}

type AgentOutput struct {
	Result struct {
		Payloads []struct {
			Text string `json:"text"`
		} `json:"payloads"`
	} `json:"result"`
}

// ──────────────────────────────────────────────
// 取得 Direct Session ID
// ──────────────────────────────────────────────

func getDirectSessionID() (string, error) {
	out, err := exec.Command(cfg.OpenclawBin, "sessions", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("執行 openclaw sessions 失敗: %w", err)
	}

	var data SessionsOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return "", fmt.Errorf("解析 sessions JSON 失敗: %w", err)
	}

	for _, s := range data.Sessions {
		if s.Kind == "direct" {
			return s.SessionID, nil
		}
	}
	return "", fmt.Errorf("找不到 kind=direct 的 session")
}

// ──────────────────────────────────────────────
// 將使用者訊息轉發給 OpenClaw Agent 並取得回覆
// ──────────────────────────────────────────────

func queryAgent(sessionID, userMsg string) (string, error) {
	out, err := exec.Command(
		cfg.OpenclawBin, "agent",
		"--session-id", sessionID,
		"--message", userMsg,
		"--json",
		"--timeout", "30",
	).Output()
	if err != nil {
		return "", fmt.Errorf("執行 openclaw agent 失敗: %w", err)
	}

	var data AgentOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return "", fmt.Errorf("解析 agent JSON 失敗: %w", err)
	}

	if len(data.Result.Payloads) == 0 || data.Result.Payloads[0].Text == "" {
		return "(empty reply)", nil
	}
	return data.Result.Payloads[0].Text, nil
}

// ──────────────────────────────────────────────
// 送出回應（建立獨立的 WebSocket 連線）
// ──────────────────────────────────────────────

func sendResponse(resp OutgoingMessage) error {
	conn, _, err := websocket.DefaultDialer.Dial(cfg.WebsocketURL, nil)
	if err != nil {
		return fmt.Errorf("sendResponse 連線失敗: %w", err)
	}
	defer conn.Close()

	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("序列化回應失敗: %w", err)
	}

	return conn.WriteMessage(websocket.TextMessage, payload)
}

// ──────────────────────────────────────────────
// 處理單一收到的訊息
// ──────────────────────────────────────────────

func processMessage(msg IncomingMessage) {
	// 只處理發給自己的訊息
	if msg.ReplyTo != cfg.UserID {
		return
	}
	if msg.Message == "" {
		return
	}

	// 1. 取得 Direct Session ID
	sessID, err := getDirectSessionID()
	if err != nil {
		log.Printf("⚠️ 取得 session 失敗: %v", err)
		_ = sendResponse(OutgoingMessage{
			Channel:     cfg.Channel,
			Message:     "⚠️ 無法取得會話 ID",
			UserID:      cfg.UserID,
			DisplayName: cfg.DisplayName,
			Data:        "",
			ReplyTo:     msg.UserID,
			Type:        "response",
		})
		return
	}

	// 2. 轉發訊息給 Agent 取得回覆
	replyText, err := queryAgent(sessID, msg.Message)
	if err != nil {
		log.Printf("⚠️ 查詢 agent 失敗: %v", err)
		replyText = fmt.Sprintf("⚠️ 轉發失敗：%v", err)
	}

	// 3. 透過 WebSocket 送出回覆
	if err := sendResponse(OutgoingMessage{
		Channel:     cfg.Channel,
		Message:     replyText,
		UserID:      cfg.UserID,
		DisplayName: cfg.DisplayName,
		Data:        "",
		ReplyTo:     msg.UserID,
		Type:        "response",
	}); err != nil {
		log.Printf("⚠️ 送出回應失敗: %v", err)
	}
}

// ──────────────────────────────────────────────
// 主要監聽迴圈（自動重連）
// ──────────────────────────────────────────────

func listen() {
	for {
		if err := connectAndListen(); err != nil {
			log.Printf("連線中斷: %v，1 秒後重連…", err)
		}
		time.Sleep(1 * time.Second)
	}
}

func connectAndListen() error {
	log.Printf("正在連線到 %s", cfg.WebsocketURL)
	conn, _, err := websocket.DefaultDialer.Dial(cfg.WebsocketURL, nil)
	if err != nil {
		return fmt.Errorf("連線失敗: %w", err)
	}
	defer conn.Close()
	log.Println("WebSocket 連線成功，開始監聽…")

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("讀取訊息失敗: %w", err)
		}

		var msg IncomingMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			// 忽略非 JSON 格式訊息
			continue
		}

		// 在 goroutine 中非同步處理，避免阻塞主迴圈
		go processMessage(msg)
	}
}

// ──────────────────────────────────────────────
// 入口點
// ──────────────────────────────────────────────

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 載入 .env 設定
	cfg = loadConfig()

	log.Printf("WebSocket Bot 啟動中… UserID=%s Channel=%s", cfg.UserID, cfg.Channel)
	listen() // 永遠不會返回
}
