package notify

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"btc-agent/internal/reportio"
)

type TelegramAPI interface {
	Send(ctx context.Context, token, chatID, text string) (SendResult, error)
	Delete(ctx context.Context, token, chatID string, messageID int) error
}

type RealTelegramAPI struct{}

func (RealTelegramAPI) Send(ctx context.Context, token, chatID, text string) (SendResult, error) {
	return TelegramSend(ctx, token, chatID, text)
}

func (RealTelegramAPI) Delete(ctx context.Context, token, chatID string, messageID int) error {
	return TelegramDelete(ctx, token, chatID, messageID)
}

type TelegramManager struct {
	ReportDir string
	API       TelegramAPI
	mu        sync.Mutex
}

func NewTelegramManager(reportDir string, api TelegramAPI) *TelegramManager {
	if reportDir == "" {
		reportDir = "reports"
	}
	if api == nil {
		api = RealTelegramAPI{}
	}
	return &TelegramManager{ReportDir: reportDir, API: api}
}

func (m *TelegramManager) Send(ctx context.Context, token, chatID, label, text string) (SendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.SaveCopy(label, text); err != nil {
		log.Printf("telegram copy warning [%s]: %v", label, err)
	}

	state, err := m.loadState()
	if err != nil {
		log.Printf("telegram state load warning [%s]: %v", label, err)
		state = map[string]int{}
	}

	oldID := state[label]
	if oldID != 0 {
		if err := m.API.Delete(ctx, token, chatID, oldID); err != nil {
			log.Printf("telegram delete old [%s] msg_id=%d: %v", label, oldID, err)
		}
	}

	result, err := m.API.Send(ctx, token, chatID, text)
	if err != nil {
		return result, err
	}
	if result.MessageID != 0 {
		state[label] = result.MessageID
		if err := m.saveState(state); err != nil {
			log.Printf("telegram state save warning [%s]: %v", label, err)
		}
	}
	return result, nil
}

func (m *TelegramManager) SaveCopy(label, text string) error {
	return reportio.WriteMarkdown(m.ReportDir, "telegram_"+reportio.SafeLabel(label)+"_latest.md", text)
}

func (m *TelegramManager) loadState() (map[string]int, error) {
	path := filepath.Join(m.ReportDir, "telegram_state.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	state := map[string]int{}
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func (m *TelegramManager) saveState(state map[string]int) error {
	return reportio.WriteJSON(m.ReportDir, "telegram_state.json", state)
}
