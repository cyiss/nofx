package hyperliquid

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	hl "github.com/sonirico/go-hyperliquid"
	"io"
	"net/http"
	"nofx/trader/types"
	"strings"
	"testing"
)

type securityTransport func(*http.Request) (*http.Response, error)

func (f securityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// All requests, including the SDK and hard-coded XYZ URLs, are intercepted in memory.
func securityTrader(t testing.TB, xyzFailure bool, leverageFailure bool, orders *int, errorEnvelope ...bool) *HyperliquidTrader {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = securityTransport(func(r *http.Request) (*http.Response, error) {
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		status, body := 200, `{}`
		if r.URL.Path == "/exchange" {
			action, _ := request["action"].(map[string]interface{})
			if action["type"] == "updateLeverage" && leverageFailure {
				status, body = 500, `injected leverage failure`
				if len(errorEnvelope) > 0 && errorEnvelope[0] {
					status, body = 200, `{"status":"err","response":"injected leverage rejection"}`
				}
			} else {
				if action["type"] == "order" || action["type"] == "cancel" {
					*orders++
				}
				body = `{"status":"ok","response":{"type":"order","data":{"statuses":[{"resting":{"oid":1}}]}}}`
			}
		} else {
			switch request["type"] {
			case "clearinghouseState":
				body = `{"marginSummary":{"accountValue":"100","totalMarginUsed":"0"},"crossMarginSummary":{"accountValue":"100","totalMarginUsed":"0"},"withdrawable":"100","assetPositions":[]}`
				if request["dex"] == "xyz" && xyzFailure {
					status, body = 503, `injected XYZ outage`
				}
			case "spotClearinghouseState":
				body = `{"balances":[]}`
			case "openOrders":
				body = `[]`
				if leverageFailure {
					coin := "BTC"
					if request["dex"] == "xyz" {
						coin = "xyz:TSLA"
					}
					body = fmt.Sprintf(`[{"coin":%q,"oid":123}]`, coin)
				}
			case "allMids":
				body = `{"BTC":"100","xyz:TSLA":"100"}`
			case "meta":
				body = `{"universe":[{"name":"TSLA","szDecimals":2,"maxLeverage":10}]}`
			default:
				t.Fatalf("unexpected request (network blocked): %v", request)
			}
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	key, err := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	meta := &hl.Meta{Universe: []hl.AssetInfo{{Name: "BTC", SzDecimals: 2}, {Name: "xyz:TSLA", SzDecimals: 2}}}
	ex := hl.NewExchange(context.Background(), key, "https://mock.invalid", meta, "", "0x1111111111111111111111111111111111111111", &hl.SpotMeta{}, nil, leverageResponseOption())
	return &HyperliquidTrader{exchange: ex, ctx: context.Background(), meta: meta, privateKey: key, xyzMeta: &xyzDexMeta{Universe: []xyzAssetInfo{{Name: "xyz:TSLA", SzDecimals: 2}}}}
}

func TestIncompleteXYZSnapshotFailsClosed(t *testing.T) {
	orders := 0
	trader := securityTrader(t, true, false, &orders)
	positions, err := trader.GetPositions()
	if err == nil || positions != nil {
		t.Errorf("partial positions escaped: positions=%v err=%v", positions, err)
	}
	balance, err := trader.GetBalance()
	if err == nil || balance != nil {
		t.Errorf("partial balance escaped: balance=%v err=%v", balance, err)
	}
}

func TestLeverageFailurePreventsOrders(t *testing.T) {
	for _, symbol := range []string{"BTCUSDT", "xyz:TSLA"} {
		for _, kind := range []string{"long", "short", "limit"} {
			t.Run(symbol+"/"+kind, func(t *testing.T) {
				orders := 0
				trader := securityTrader(t, false, true, &orders)
				var err error
				switch kind {
				case "long":
					_, err = trader.OpenLong(symbol, 1, 3)
				case "short":
					_, err = trader.OpenShort(symbol, 1, 3)
				case "limit":
					_, err = trader.PlaceLimitOrder(&types.LimitOrderRequest{Symbol: symbol, Quantity: 1, Price: 100, Leverage: 3, Side: "BUY"})
				}
				if err == nil || !strings.Contains(err.Error(), "leverage") {
					t.Errorf("want leverage error; got %v", err)
				}
				if orders != 0 {
					t.Errorf("submitted %d orders after leverage failed", orders)
				}
			})
		}
	}
}

func BenchmarkCompletePositionSnapshot(b *testing.B) {
	orders := 0
	trader := securityTrader(b, false, false, &orders)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := trader.GetPositions(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSuccessfulLeverageSetupStillPlacesOrders(t *testing.T) {
	for _, symbol := range []string{"BTCUSDT", "xyz:TSLA"} {
		for _, kind := range []string{"long", "short", "limit"} {
			t.Run(symbol+"/"+kind, func(t *testing.T) {
				orders := 0
				trader := securityTrader(t, false, false, &orders)
				var err error
				switch kind {
				case "long":
					_, err = trader.OpenLong(symbol, 1, 3)
				case "short":
					_, err = trader.OpenShort(symbol, 1, 3)
				case "limit":
					_, err = trader.PlaceLimitOrder(&types.LimitOrderRequest{Symbol: symbol, Quantity: 1, Price: 100, Leverage: 3, Side: "BUY"})
				}
				if err != nil || orders != 1 {
					t.Fatalf("err=%v orders=%d", err, orders)
				}
			})
		}
	}
}

func TestLeverageRejectsHTTP200ErrorEnvelope(t *testing.T) {
	orders := 0
	trader := securityTrader(t, false, true, &orders, true)
	_, err := trader.OpenLong("xyz:TSLA", 1, 3)
	if err == nil || orders != 0 {
		t.Fatalf("API rejected leverage but err=%v orders=%d", err, orders)
	}
}

func TestLeverageAcknowledgementValidation(t *testing.T) {
	for _, body := range []string{`{"status":"err"}`, `{}`, `null`, `not-json`} {
		t.Run(body, func(t *testing.T) {
			base := securityTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})
			req, _ := http.NewRequest("POST", "https://mock.invalid/exchange", strings.NewReader(`{"action":{"type":"updateLeverage"}}`))
			resp, err := (leverageResponseTransport{base: base}).RoundTrip(req)
			if err == nil || resp != nil {
				t.Fatalf("unacknowledged setup accepted: %v", err)
			}
		})
	}
}

func TestLeverageRejectsUnexpectedHTTPStatus(t *testing.T) {
	for _, status := range []int{201, 202, 204} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			base := securityTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
			})
			req, _ := http.NewRequest("POST", "https://mock.invalid/exchange", strings.NewReader(`{"action":{"type":"updateLeverage"}}`))
			resp, err := (leverageResponseTransport{base: base}).RoundTrip(req)
			if err == nil || resp != nil {
				t.Fatalf("unexpected HTTP %d accepted", status)
			}
		})
	}
}

func BenchmarkLeverageAcknowledgement(b *testing.B) {
	base := securityTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"ok","response":{"type":"default"}}`)), Request: req}, nil
	})
	for _, checked := range []bool{false, true} {
		b.Run(fmt.Sprint("validated=", checked), func(b *testing.B) {
			var transport http.RoundTripper = base
			if checked {
				transport = leverageResponseTransport{base: base}
			}
			req, _ := http.NewRequest("POST", "https://mock.invalid/exchange", strings.NewReader(`{"action":{"type":"updateLeverage","asset":0,"isCross":true,"leverage":3},"nonce":1}`))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resp, err := transport.RoundTrip(req)
				if err != nil {
					b.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		})
	}
}
