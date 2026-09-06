package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type captureTransport struct{ payloads chan telemetryPayload }

func (c captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var payload telemetryPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		return nil, err
	}
	c.payloads <- payload
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
}

func captureTelemetry(t *testing.T, enabled bool) <-chan telemetryPayload {
	t.Helper()
	previousClient, previousHTTP := client, httpClient
	client = &Client{enabled: enabled, installationID: "stable-installation-secret"}
	payloads := make(chan telemetryPayload, 10)
	httpClient = &http.Client{Transport: captureTransport{payloads: payloads}}
	t.Cleanup(func() { client, httpClient = previousClient, previousHTTP })
	return payloads
}

func TestDisabledTelemetrySendsNothing(t *testing.T) {
	payloads := captureTelemetry(t, false)
	TrackTrade(TradeEvent{})
	TrackStartup("version")
	TrackAIUsage(AIUsageEvent{})
	select {
	case <-payloads:
		t.Fatal("disabled telemetry made an HTTP request")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOptInTelemetryExcludesFinancialAndStableIdentifiers(t *testing.T) {
	payloads := captureTelemetry(t, true)
	TrackTrade(TradeEvent{Exchange: "exchange", TradeType: "long", Symbol: "PRIVATE-SYMBOL", AmountUSD: 12345, Leverage: 20, UserID: "private-user", TraderID: "private-trader"})
	TrackStartup("v1")
	TrackAIUsage(AIUsageEvent{UserID: "private-user", TraderID: "private-trader", ModelProvider: "provider", ModelName: "private-custom-model", Channel: "native", InputTokens: 100, OutputTokens: 50})
	seenIDs := make(map[string]bool)
	for i := 0; i < 3; i++ {
		select {
		case payload := <-payloads:
			if payload.ClientID == "" || payload.ClientID == "stable-installation-secret" || seenIDs[payload.ClientID] {
				t.Error("telemetry client_id must be ephemeral per event")
			}
			seenIDs[payload.ClientID] = true
			for _, event := range payload.Events {
				for _, forbidden := range []string{"amount_usd", "leverage", "user_id", "trader_id", "installation_id", "symbol", "model_name"} {
					if _, ok := event.Params[forbidden]; ok {
						t.Errorf("%s contains private field %s", event.Name, forbidden)
					}
				}
			}
		case <-time.After(time.Second):
			t.Fatal("opt-in event was not sent")
		}
	}
}

func BenchmarkDisabledTradeTelemetry(b *testing.B) {
	previous := client
	client = &Client{}
	defer func() { client = previous }()
	for i := 0; i < b.N; i++ {
		TrackTrade(TradeEvent{})
	}
}
