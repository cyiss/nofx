package nofxos

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"nofx/mcp"
	"nofx/mcp/payment"
	"strings"
	"testing"
)

type paymentSecurityTransport func(*http.Request) (*http.Response, error)

func (f paymentSecurityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAI500DoesNotRepeatPaidRequest(t *testing.T) {
	for _, failure := range []string{"402", "500", "transport", "invalid-json", "api-failure"} {
		t.Run(failure, func(t *testing.T) {
			c, err := NewClaw402DataClient("https://mock.invalid", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", mcp.NewNoopLogger())
			if err != nil {
				t.Fatal(err)
			}
			terms, _ := json.Marshal(payment.X402v2PaymentRequired{X402Version: 2, Accepts: []payment.X402AcceptOption{{Scheme: "exact", Network: payment.BaseNetwork, Asset: payment.BaseUSDCContract, Amount: "1000", PayTo: "0x1111111111111111111111111111111111111111", MaxTimeoutSeconds: 300}}})
			paid := 0
			c.httpClient = &http.Client{Transport: paymentSecurityTransport(func(r *http.Request) (*http.Response, error) {
				status, body := 402, "challenge"
				headers := make(http.Header)
				headers.Set("Payment-Required", base64.StdEncoding.EncodeToString(terms))
				if r.Header.Get("Payment-Signature") != "" {
					paid++
					switch failure {
					case "402":
					case "500":
						status = 500
					case "transport":
						return nil, errors.New("injected connection loss after authorization")
					case "invalid-json":
						status, body = 200, "invalid-json"
					case "api-failure":
						status, body = 200, `{"success":false}`
					}
				}
				return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			client := NewClient("https://mock.invalid", "test")
			client.SetClaw402(c)
			_, err = client.GetAI500List()
			if err == nil {
				t.Fatal("expected failure")
			}
			if paid != 1 {
				t.Fatalf("one AI500 operation sent %d independently payable authorizations", paid)
			}
		})
	}
}
