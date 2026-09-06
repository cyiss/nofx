package hyperliquid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	hl "github.com/sonirico/go-hyperliquid"
)

// The SDK decodes UpdateLeverage into UserState and discards the exchange's
// status envelope. Validate that acknowledgement before the SDK can report
// success. Other actions keep the SDK's existing response handling.
func leverageResponseOption() hl.ExchangeOpt {
	return hl.ExchangeOptClientOptions(hl.ClientOptHTTPClient(&http.Client{
		Transport: leverageResponseTransport{base: http.DefaultTransport},
	}))
}

type leverageResponseTransport struct{ base http.RoundTripper }

func (t leverageResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var action struct {
		Action struct {
			Type string `json:"type"`
		} `json:"action"`
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("cannot inspect exchange action before sending")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	err = json.NewDecoder(body).Decode(&action)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("invalid exchange action: %w", err)
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil || action.Action.Type != "updateLeverage" {
		return resp, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("leverage update returned unexpected HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read leverage acknowledgement: %w", err)
	}
	var acknowledgement struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &acknowledgement); err != nil {
		return nil, fmt.Errorf("invalid leverage acknowledgement: %w", err)
	}
	if acknowledgement.Status != "ok" {
		return nil, fmt.Errorf("leverage update was not acknowledged (status %q)", acknowledgement.Status)
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return resp, nil
}
