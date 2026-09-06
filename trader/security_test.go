package trader

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"nofx/kernel"
	"nofx/store"
	"nofx/trader/types"
	"strings"
	"testing"
	"time"
)

type failClosedTrader struct {
	Trader
	positionErr, marginErr, leverageErr, balanceErr error
	orders                                          int
}

func (f *failClosedTrader) GetPositions() ([]map[string]interface{}, error) {
	return nil, f.positionErr
}
func (f *failClosedTrader) GetMarketPrice(string) (float64, error) { return 100, nil }
func (f *failClosedTrader) SetMarginMode(string, bool) error       { return f.marginErr }
func (f *failClosedTrader) SetLeverage(string, int) error          { return f.leverageErr }
func (f *failClosedTrader) SetStopLoss(string, string, float64, float64) error {
	f.orders++
	return nil
}
func (f *failClosedTrader) SetTakeProfit(string, string, float64, float64) error {
	f.orders++
	return nil
}

func TestGridAdaptersRejectLeverageFailure(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		f := &failClosedTrader{leverageErr: errors.New("injected leverage failure")}
		req := &LimitOrderRequest{Symbol: "BTCUSDT", Leverage: 3, Side: "BUY"}
		var err error
		if legacy {
			_, err = NewGridTraderAdapter(f).PlaceLimitOrder(req)
		} else {
			_, err = types.NewGridTraderAdapter(f).PlaceLimitOrder(req)
		}
		if !errors.Is(err, f.leverageErr) || f.orders != 0 {
			t.Errorf("legacy=%v: err=%v orders=%d", legacy, err, f.orders)
		}
	}
}
func securityGrid(f *failClosedTrader) *AutoTrader {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource.SourceType = "static"
	cfg.GridConfig = &store.GridStrategyConfig{Symbol: "BTCUSDT", GridCount: 3, TotalInvestment: 100, Leverage: 3, UpperPrice: 110, LowerPrice: 90}
	return &AutoTrader{trader: f, config: AutoTraderConfig{StrategyConfig: &cfg}, gridState: NewGridState(cfg.GridConfig)}
}
func TestGridSetupFailureRemainsUninitialized(t *testing.T) {
	for _, margin := range []bool{false, true} {
		f := &failClosedTrader{}
		if margin {
			f.marginErr = errors.New("injected margin failure")
		} else {
			f.leverageErr = errors.New("injected leverage failure")
		}
		at := securityGrid(f)
		if err := at.InitializeGrid(); err == nil {
			t.Error("expected setup failure")
		}
		if at.gridState.IsInitialized {
			t.Error("failed setup left grid ready to trade")
		}
	}
}
func TestGridPositionLimitFailsClosed(t *testing.T) {
	at := securityGrid(&failClosedTrader{positionErr: errors.New("XYZ state unavailable")})
	allowed, _, _ := at.checkTotalPositionLimit("BTCUSDT", 10)
	if allowed {
		t.Fatal("allowed new exposure with unknown position state")
	}
}
func TestOpenDecisionsRejectIncompletePositions(t *testing.T) {
	for _, action := range []string{"open_long", "open_short"} {
		f := &failClosedTrader{positionErr: errors.New("XYZ state unavailable")}
		at := securityGrid(f)
		err := at.executeDecisionWithRecord(&kernel.Decision{Action: action, Symbol: "BTCUSDT"}, &store.DecisionAction{})
		if !errors.Is(err, f.positionErr) {
			t.Fatalf("got %v", err)
		}
	}
}

func (f *failClosedTrader) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{"totalEquity": 1000.0, "availableBalance": 1000.0}, f.balanceErr
}
func (f *failClosedTrader) OpenLong(string, float64, int) (map[string]interface{}, error) {
	f.orders++
	return nil, errors.New("order unexpectedly reached")
}
func (f *failClosedTrader) OpenShort(string, float64, int) (map[string]interface{}, error) {
	f.orders++
	return nil, errors.New("order unexpectedly reached")
}
func (f *failClosedTrader) GetOpenOrders(string) ([]OpenOrder, error) { return nil, nil }

type marketSecurityTransport func(*http.Request) (*http.Response, error)

func (f marketSecurityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func mockSecurityMarket(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = marketSecurityTransport(func(r *http.Request) (*http.Response, error) {
		var req map[string]interface{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		body := `{}`
		if req["type"] == "candleSnapshot" {
			candles := make([]map[string]interface{}, 100)
			for i := range candles {
				candles[i] = map[string]interface{}{"t": time.Now().Add(time.Duration(i-100) * 5 * time.Minute).UnixMilli(), "T": time.Now().UnixMilli(), "s": "xyz:TSLA", "i": "5m", "o": "100", "h": "102", "l": "99", "c": fmt.Sprint(100 + i%2), "v": "100", "n": 10}
			}
			data, _ := json.Marshal(candles)
			body = string(data)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
}
func TestOpenDecisionsRejectMarginFailure(t *testing.T) {
	mockSecurityMarket(t)
	for _, action := range []string{"open_long", "open_short"} {
		f := &failClosedTrader{marginErr: errors.New("injected margin failure")}
		at := securityGrid(f)
		sl, tp := 90.0, 110.0
		if action == "open_short" {
			sl, tp = 110, 90
		}
		err := at.executeDecisionWithRecord(&kernel.Decision{Action: action, Symbol: "xyz:TSLA", Leverage: 3, PositionSizeUSD: 100, StopLoss: sl, TakeProfit: tp}, &store.DecisionAction{})
		if !errors.Is(err, f.marginErr) || f.orders != 0 {
			t.Fatalf("%s: err=%v orders=%d", action, err, f.orders)
		}
	}
}
func TestGridSyncPreservesUnknownPendingOrders(t *testing.T) {
	f := &failClosedTrader{positionErr: errors.New("XYZ state unavailable")}
	at := securityGrid(f)
	at.gridState.Levels = []kernel.GridLevelInfo{{State: "pending", OrderID: "pending-1", OrderQuantity: 1, Price: 100}}
	at.gridState.OrderBook["pending-1"] = 0
	at.syncGridState()
	if at.gridState.Levels[0].State != "pending" {
		t.Fatal("unknown position state cleared pending exposure")
	}
}

func TestGridContextRejectsIncompleteSnapshot(t *testing.T) {
	mockSecurityMarket(t)
	for _, balance := range []bool{false, true} {
		f := &failClosedTrader{}
		if balance {
			f.balanceErr = errors.New("XYZ balance unavailable")
		} else {
			f.positionErr = errors.New("XYZ positions unavailable")
		}
		at := securityGrid(f)
		at.config.StrategyConfig.GridConfig.Symbol = "xyz:TSLA"
		ctx, err := at.buildGridContext()
		if err == nil || ctx != nil {
			t.Errorf("balance=%v: incomplete snapshot reached decision context", balance)
		}
	}
}
