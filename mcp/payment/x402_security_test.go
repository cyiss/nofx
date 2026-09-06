package payment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"nofx/mcp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRepeated402DoesNotAuthorizeAnotherPayment(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Payment-Required", "changed-terms")
				w.WriteHeader(http.StatusPaymentRequired)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			signs := 0
			sign := func(string) (string, error) { signs++; return fmt.Sprint("authorization-", signs), nil }
			build := func() (*http.Request, error) { return http.NewRequest(http.MethodPost, server.URL, nil) }
			var err error
			if stream {
				var resp *http.Response
				resp, err = DoX402RequestStream(ctx, server.Client(), build, sign, "test", mcp.NewNoopLogger())
				if resp != nil {
					resp.Body.Close()
				}
			} else {
				_, err = DoX402Request(ctx, server.Client(), build, sign, "test", mcp.NewNoopLogger())
			}
			if err == nil {
				t.Fatal("ambiguous payment status must fail")
			}
			if signs != 1 {
				t.Fatalf("signed %d independent authorizations for one call; want 1", signs)
			}
			if requests.Load() != 2 {
				t.Fatalf("got %d requests, want initial challenge and one authorized request", requests.Load())
			}
		})
	}
}

func TestSinglePaymentAuthorizationSucceeds(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Header.Get("Payment-Signature") == "" {
					w.Header().Set("Payment-Required", "terms")
					w.WriteHeader(402)
					return
				}
				if r.Header.Get("Payment-Signature") != "single-authorization" || r.Header.Get("X-Payment") != "single-authorization" {
					t.Error("authorization header changed")
				}
				_, _ = w.Write([]byte("success"))
			}))
			defer server.Close()
			signs := 0
			sign := func(string) (string, error) { signs++; return "single-authorization", nil }
			build := func() (*http.Request, error) { return http.NewRequest("POST", server.URL, nil) }
			if stream {
				resp, err := DoX402RequestStream(context.Background(), server.Client(), build, sign, "test", mcp.NewNoopLogger())
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
			} else {
				body, err := DoX402Request(context.Background(), server.Client(), build, sign, "test", mcp.NewNoopLogger())
				if err != nil || string(body) != "success" {
					t.Fatalf("body=%s err=%v", body, err)
				}
			}
			if signs != 1 || requests.Load() != 2 {
				t.Fatalf("signs=%d requests=%d", signs, requests.Load())
			}
		})
	}
}

type faultPaymentTransport func(*http.Request) (*http.Response, error)

func (f faultPaymentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failedPaymentBody struct{}

func (failedPaymentBody) Read([]byte) (int, error) { return 0, errors.New("injected read failure") }
func (failedPaymentBody) Close() error             { return nil }
func TestPaidErrorsAreNonRetryable(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, failure := range []string{"402", "500", "transport", "read"} {
			t.Run(fmt.Sprint(stream, "/", failure), func(t *testing.T) {
				client := &http.Client{Transport: faultPaymentTransport(func(r *http.Request) (*http.Response, error) {
					headers := make(http.Header)
					headers.Set("Payment-Required", "terms")
					resp := &http.Response{StatusCode: 402, Header: headers, Body: io.NopCloser(strings.NewReader("failure")), Request: r}
					if r.Header.Get("Payment-Signature") != "" {
						switch failure {
						case "500":
							resp.StatusCode = 500
						case "transport":
							return nil, errors.New("injected connection failure")
						case "read":
							resp.StatusCode = 500
							resp.Body = failedPaymentBody{}
						}
					}
					return resp, nil
				})}
				build := func() (*http.Request, error) { return http.NewRequest("GET", "https://mock.invalid", nil) }
				sign := func(string) (string, error) { return "authorization", nil }
				var err error
				if stream {
					_, err = DoX402RequestStream(context.Background(), client, build, sign, "test", mcp.NewNoopLogger())
				} else {
					_, err = DoX402Request(context.Background(), client, build, sign, "test", mcp.NewNoopLogger())
				}
				if !errors.Is(err, ErrPaymentOutcomeUnknown) {
					t.Fatalf("paid %s error is replayable: %v", failure, err)
				}
			})
		}
	}
}
