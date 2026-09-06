package telemetry

import (
 "encoding/json"
 "io"
 "net/http"
 "strings"
 "testing"
 "time"
)

type captureTransport func(*http.Request) (*http.Response, error)
func (f captureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercises a reviewed copy of the real telemetry source with every HTTP request
// intercepted in memory. It cannot connect to Google or a trading service.
func TestTradeTelemetrySendsFinancialMetadata(t *testing.T) {
 Init(true, "audit-installation")
 captured := make(chan *http.Request, 1)
 var payload telemetryPayload
 httpClient = &http.Client{Transport: captureTransport(func(r *http.Request) (*http.Response, error) {
  if err := json.NewDecoder(r.Body).Decode(&payload); err != nil { return nil, err }
  captured <- r
  return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
 })}
 TrackTrade(TradeEvent{Exchange: "test-exchange", TradeType: "open_long", Symbol: "TESTUSDT", AmountUSD: 123.45, Leverage: 7, UserID: "audit-user", TraderID: "audit-trader"})
 select {
 case r := <-captured:
  if r.URL.Host != "www.google-analytics.com" { t.Fatalf("unexpected destination: %s", r.URL.Host) }
  if payload.ClientID != "audit-installation" || len(payload.Events) != 1 { t.Fatalf("unexpected payload: %#v", payload) }
  p := payload.Events[0].Params
  for key, expected := range map[string]interface{}{"symbol": "TESTUSDT", "amount_usd": 123.45, "leverage": float64(7), "user_id": "audit-user", "trader_id": "audit-trader"} {
   if p[key] != expected { t.Errorf("%s: got %v, want %v", key, p[key], expected) }
  }
  t.Log("Confirmed: amount, symbol, leverage, user and trader IDs sent to GA endpoint; HTTP intercepted locally.")
 case <-time.After(3*time.Second):
  t.Fatal("no captured telemetry request")
 }
 SetEnabled(false)
 TrackTrade(TradeEvent{Symbol: "MUST_NOT_SEND"})
 select {
 case <-captured: t.Fatal("disabled telemetry sent data")
 case <-time.After(20*time.Millisecond):
 }
}
