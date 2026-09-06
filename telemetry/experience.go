// Package telemetry handles product telemetry
package telemetry

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const (
	telemetryEndpoint = "https://www.google-analytics.com/mp/collect"
	tid               = "G-14J8SY6F0J"
	tk                = "sgPLmshGTPiF-X57rzEIKA"
)

var (
	client     *Client
	clientOnce sync.Once
	httpClient = &http.Client{Timeout: 5 * time.Second}
)

type Client struct {
	enabled        bool
	installationID string
	mu             sync.RWMutex
}

type TradeEvent struct {
	Exchange  string
	TradeType string
	Symbol    string
	AmountUSD float64
	Leverage  int
	UserID    string
	TraderID  string
}

type AIUsageEvent struct {
	UserID        string
	TraderID      string
	ModelProvider string // openai, deepseek, anthropic, etc.
	ModelName     string // gpt-4o, deepseek-chat, claude-3, etc.
	Channel       string // payment channel: "claw402" or "native"
	InputTokens   int
	OutputTokens  int
}

type telemetryPayload struct {
	ClientID string           `json:"client_id"`
	Events   []telemetryEvent `json:"events"`
}

type telemetryEvent struct {
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

func Init(enabled bool, installationID string) {
	clientOnce.Do(func() {
		client = &Client{
			enabled:        enabled,
			installationID: installationID,
		}
	})
}

func SetInstallationID(id string) {
	if client == nil {
		return
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	client.installationID = id
}

func GetInstallationID() string {
	if client == nil {
		return ""
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	return client.installationID
}

func SetEnabled(enabled bool) {
	if client == nil {
		return
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	client.enabled = enabled
}

func IsEnabled() bool {
	if client == nil {
		return false
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	return client.enabled
}

func TrackTrade(event TradeEvent) {
	if client == nil || !IsEnabled() {
		return
	}

	// Send asynchronously to not block trading
	go func() {
		_ = sendTradeEvent(event)
	}()
}

// sendTradeEvent deliberately excludes financial values and account identifiers.
func sendTradeEvent(event TradeEvent) error {
	return sendEvent("trade", map[string]interface{}{
		"exchange":   event.Exchange,
		"trade_type": event.TradeType,
	})
}

func sendEvent(name string, params map[string]interface{}) error {
	// Recheck after asynchronous dispatch so queued events respect opt-out.
	if !IsEnabled() {
		return nil
	}
	// GA4 requires client_id. Use a fresh random value per event, never the
	// persisted installation ID, user ID or trader ID.
	var eventID [16]byte
	if _, err := rand.Read(eventID[:]); err != nil {
		return err
	}
	params["engagement_time_msec"] = 1

	payload := telemetryPayload{
		ClientID: hex.EncodeToString(eventID[:]),
		Events: []telemetryEvent{
			{
				Name:   name,
				Params: params,
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := telemetryEndpoint + "?measurement_id=" + tid + "&api_secret=" + tk
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func TrackStartup(version string) {
	if client == nil || !IsEnabled() {
		return
	}

	go func() {
		_ = sendEvent("app_startup", map[string]interface{}{"version": version})
	}()
}

func TrackAIUsage(event AIUsageEvent) {
	if client == nil || !IsEnabled() {
		return
	}

	go func() {
		_ = sendEvent("ai_usage", map[string]interface{}{
			"model_provider": event.ModelProvider,
			"channel":        event.Channel,
			"input_tokens":   event.InputTokens,
			"output_tokens":  event.OutputTokens,
			"total_tokens":   event.InputTokens + event.OutputTokens,
		})
	}()
}
