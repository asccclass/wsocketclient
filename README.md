# WebSocket Client Bot (pcaiwebsocket)

`pcaiwebsocket` 是一個基於 Go 語言開發的 WebSocket 機器人，專為 PCAI (Personal Cloud AI) 架構設計。它能夠監聽 WebSocket 訊息，並將特定訊息轉發給外部的 `openclaw` Agent 進行處理，最後將回覆送回 WebSocket 頻道。

## 功能特色

- **自動重連**：當 WebSocket 連線中斷時，機器人會自動嘗試重新連線（每隔 1 秒）。
- **非同步處理**：使用 Goroutine 處理訊息，避免阻塞主要的監聽迴圈。
- **OpenClaw 整合**：透過執行 `openclaw` 命令列工具與 AI Agent 溝通。
- **對話過濾**：僅處理指定給機器人 (`reply_to` 匹配 `BOT_USER_ID`) 的訊息。

## 前置需求

1. **Go 語言環境**：建議版本 1.25.5 或更高。
2. **OpenClaw CLI**：必須安裝 `openclaw` 二進位檔，並確保其在設定的路徑下可執行。
3. **PCAI WebSocket 伺服器**：機器人需要連接的後端服務。

## 安裝步驟

1. 複製此專案：
   ```bash
   git clone <repository-url>
   cd wsocketclient
   ```

2. 安裝依賴套件：
   ```bash
   go mod download
   ```

## 配置說明

在專案根目錄下建立 `.env` 檔案（可參考 `envfile.example`）：

```env
BOT_USER_ID=your_bot_id          # 機器人的唯一識別碼
BOT_DISPLAY_NAME=BotName         # 顯示名稱
BOT_CHANNEL=general              # 回應的頻道名稱
WEBSOCKET_BASE_URL=wss://...     # WebSocket 基礎網址
OPENCLAW_BIN=/path/to/openclaw   # openclaw 執行檔的路徑
```

### 環境變數說明

| 變數名稱 | 說明 |
| :--- | :--- |
| `BOT_USER_ID` | 機器人的 User ID，用於識別發送給自己的訊息。 |
| `BOT_DISPLAY_NAME` | 機器人發送回覆時使用的顯示名稱。 |
| `BOT_CHANNEL` | 機器人回覆訊息時所屬的頻道。 |
| `WEBSOCKET_BASE_URL` | WebSocket 伺服器的 URL（程式會自動加上 `?user=...` 參數）。 |
| `OPENCLAW_BIN` | `openclaw` 執行檔的完整路徑。 |

## 使用方法

執行以下指令啟動機器人：

```bash
go run websocketclient.go
```

程式啟動後會顯示：
```text
WebSocket Bot 啟動中… UserID=... Channel=...
正在連線到 wss://...
WebSocket 連線成功，開始監聽…
```

## 運作邏輯

1. **連線**：連線至指定的 WebSocket URL。
2. **監聽**：持續讀取伺服器推播的訊息。
3. **過濾**：檢查訊息的 `reply_to` 是否等於 `BOT_USER_ID`。
4. **處理**：
   - 呼叫 `openclaw sessions --json` 取得 `kind=direct` 的 session ID。
   - 呼叫 `openclaw agent --session-id <ID> --message <MSG> --json` 取得 AI 系統的回覆。
5. **回饋**：建立一個臨時的 WebSocket 連線將結果送回。

## 授權條款

[MIT License](LICENSE) (或根據您的專案需求修改)
