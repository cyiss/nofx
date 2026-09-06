package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"nofx/auth"
	"nofx/manager"
	"nofx/store"
)

func securityServer(t testing.TB) (*Server, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "security.db")
	st, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	auth.SetJWTSecret("security-test-only-secret-with-more-than-32-bytes")
	return &Server{store: st, traderManager: manager.NewTraderManager()}, path
}
func securityRequest(s *Server, handler gin.HandlerFunc, body, token string, protected bool) *httptest.ResponseRecorder {
	r := gin.New()
	if protected {
		r.Use(s.authMiddleware())
	}
	r.POST("/", handler)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func securityUser(t testing.TB, s *Server) string {
	t.Helper()
	hash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.User().Create(&store.User{ID: "owner", Email: "owner@example.test", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	return securityLogin(t, s, "old-password")
}
func securityLogin(t testing.TB, s *Server, password string) string {
	t.Helper()
	w := securityRequest(s, s.handleLogin, `{"email":"owner@example.test","password":"`+password+`"}`, "", false)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.Token
}
func securityProbe(s *Server, token string) int {
	return securityRequest(s, func(c *gin.Context) { c.Status(200) }, "{}", token, true).Code
}
func TestSecurityPasswordChangeRequiresReauthentication(t *testing.T) {
	s, _ := securityServer(t)
	token := securityUser(t, s)
	for _, body := range []string{`{"new_password":"new-password"}`, `{"old_password":"incorrect","new_password":"new-password"}`} {
		w := securityRequest(s, s.handleChangePassword, body, token, true)
		if w.Code == 200 {
			t.Errorf("password changed without valid old password: %s", body)
		}
	}
}
func TestSecurityPasswordRevocationSurvivesRestart(t *testing.T) {
	s, path := securityServer(t)
	token := securityUser(t, s)
	if err := s.store.User().UpdatePassword("owner", dummyPasswordHash); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s.store = reopened
	if got := securityProbe(s, token); got != 401 {
		t.Fatalf("old token after password reset/restart: %d", got)
	}
}
func TestSecurityLogoutRevokesOtherSessionsDurably(t *testing.T) {
	s, path := securityServer(t)
	token := securityUser(t, s)
	// A separately signed token simulates another stolen session and bypasses the old raw-token blacklist.
	other, err := auth.GenerateJWT("owner", "other@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	if w := securityRequest(s, s.handleLogout, "{}", token, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s.store.Close()
	reopened, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s.store = reopened
	if got := securityProbe(s, other); got != 401 {
		t.Fatalf("other token after logout/restart: %d", got)
	}
	fresh := securityLogin(t, s, "old-password")
	if got := securityProbe(s, fresh); got != 200 {
		t.Fatalf("fresh login: %d", got)
	}
}
func TestSecurityHiddenBatch(t *testing.T) {
	s, _ := securityServer(t)
	if err := s.store.Trader().Create(&store.Trader{ID: "hidden", UserID: "owner", Name: "Private", AIModelID: "m", ExchangeID: "e", InitialBalance: 100}); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Trader().UpdateShowInCompetition("owner", "hidden", false); err != nil {
		t.Fatal(err)
	}
	result := s.getEquityHistoryForTraders([]string{"hidden", "missing"}, 0)
	histories := result["histories"].(map[string]interface{})
	if len(histories) != 0 {
		t.Fatalf("private/missing IDs exposed: %v", histories)
	}
	if err := s.store.Trader().Create(&store.Trader{ID: "public", UserID: "owner", Name: "Public", AIModelID: "m", ExchangeID: "e", InitialBalance: 100}); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Equity().Save(&store.EquitySnapshot{TraderID: "public", TotalEquity: 110}); err != nil {
		t.Fatal(err)
	}
	mixed := s.getEquityHistoryForTraders([]string{"hidden", "public"}, 0)["histories"].(map[string]interface{})
	if len(mixed) != 1 || mixed["public"] == nil {
		t.Fatalf("public history not preserved: %v", mixed)
	}

	// Private config must be rejected before consulting runtime state at all.
	s.traderManager = nil
	r := gin.New()
	r.GET("/:id", s.handleGetPublicTraderConfig)
	for _, id := range []string{"hidden", "missing"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/"+id, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("public config %s: %d", id, w.Code)
		}
	}
}
func TestSecurityOnboardingDoesNotAdoptOrphan(t *testing.T) {
	s, _ := securityServer(t)
	// Public disposable test key; never funded or sent to any RPC.
	key := "0x" + strings.Repeat("1", 64)
	if err := s.store.AIModel().Update("former-owner", "claw402", true, key, "", ""); err != nil {
		t.Fatal(err)
	}
	actual, _, _, reused, err := s.resolveBeginnerWallet("new-owner")
	if err != nil {
		t.Fatal(err)
	}
	if reused || actual == key {
		t.Fatal("new account inherited former owner's private key")
	}
	models, err := s.store.AIModel().List("former-owner")
	if err != nil || len(models) != 1 {
		t.Fatalf("orphan ownership changed: %v", err)
	}
}
func TestSecurityForwardedHeadersDoNotBypassRateLimit(t *testing.T) {
	s := NewServer(manager.NewTraderManager(), nil, nil, 0)
	s.router.GET("/security-rate-test", rateLimitMiddleware(newIPRateLimiter(0, 1)), func(c *gin.Context) { c.Status(200) })
	for i, ip := range []string{"198.51.100.1", "198.51.100.2"} {
		req := httptest.NewRequest("GET", "/security-rate-test", nil)
		req.RemoteAddr = "192.0.2.1:12345"
		req.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		want := 200
		if i == 1 {
			want = 429
		}
		if w.Code != want {
			t.Errorf("request %d: got %d want %d", i, w.Code, want)
		}
	}
}

func TestSecurityConcurrentFirstRegistration(t *testing.T) {
	s, path := securityServer(t)
	secondStore, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	servers := []*Server{s, {store: secondStore}}
	start := make(chan struct{})
	results := make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func(i int) {
			<-start
			server := servers[i%len(servers)]
			w := securityRequest(server, server.handleRegister, fmt.Sprintf(`{"email":"user%d@example.test","password":"test-password"}`, i), "", false)
			results <- w.Code
		}(i)
	}
	close(start)
	success := 0
	for i := 0; i < 8; i++ {
		code := <-results
		if code == 200 {
			success++
		} else if code != http.StatusForbidden {
			t.Errorf("concurrent setup returned unexpected status %d", code)
		}
	}
	count, err := s.store.User().Count()
	if err != nil {
		t.Fatal(err)
	}
	if success != 1 || count != 1 {
		t.Fatalf("concurrent setup created %d users (%d successful responses)", count, success)
	}
}

type securityDenyTransport struct{}

func (securityDenyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("external HTTP disabled in security tests")
}
func TestSecurityOnboardingDoesNotPersistGlobalWallet(t *testing.T) {
	s, _ := securityServer(t)
	t.Chdir(t.TempDir())
	previousTransport := http.DefaultTransport
	http.DefaultTransport = securityDenyTransport{}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	t.Setenv("CLAW402_WALLET_KEY", "prior-key")
	t.Setenv("CLAW402_WALLET_ADDRESS", "prior-address")
	r := gin.New()
	r.POST("/", func(c *gin.Context) { c.Set("user_id", "owner"); s.handleBeginnerOnboarding(c) })
	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("onboarding: %d %s", w.Code, w.Body)
	}
	if os.Getenv("CLAW402_WALLET_KEY") != "prior-key" || os.Getenv("CLAW402_WALLET_ADDRESS") != "prior-address" {
		t.Error("user wallet leaked into global environment")
	}
	if _, err := os.Stat(".env"); !os.IsNotExist(err) {
		t.Error("onboarding wrote global plaintext .env")
	}
	var response beginnerOnboardingResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.PrivateKey == "" || response.ConfiguredModelID == "" || response.EnvSaved {
		t.Fatal("wallet must remain available from owned model only")
	}
}

func BenchmarkSecurityAuthMiddleware(b *testing.B) {
	s, _ := securityServer(b)
	token := securityUser(b, s)
	r := gin.New()
	r.Use(s.authMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			b.Fatal(w.Code)
		}
	}
}

func TestSecurityPasswordChangeRevokesOldSession(t *testing.T) {
	s, _ := securityServer(t)
	token := securityUser(t, s)
	w := securityRequest(s, s.handleChangePassword, `{"old_password":"old-password","new_password":"new-password"}`, token, true)
	if w.Code != 200 {
		t.Fatalf("valid change failed: %d %s", w.Code, w.Body)
	}
	if got := securityProbe(s, token); got != 401 {
		t.Fatalf("old session after change: %d", got)
	}
	fresh := securityLogin(t, s, "new-password")
	if got := securityProbe(s, fresh); got != 200 {
		t.Fatalf("new session: %d", got)
	}
}
func TestSecurityDeletedUserTokenRejected(t *testing.T) {
	s, _ := securityServer(t)
	token := securityUser(t, s)
	if err := s.store.User().DeleteAll(); err != nil {
		t.Fatal(err)
	}
	if got := securityProbe(s, token); got != 401 {
		t.Fatalf("deleted account still authorized: %d", got)
	}
}

func TestSecurityListenerAddress(t *testing.T) {
	for _, tc := range []struct{ host, want string }{{"", "127.0.0.1:-1"}, {"0.0.0.0", "0.0.0.0:-1"}, {"::1", "[::1]:-1"}} {
		t.Run(tc.host, func(t *testing.T) {
			t.Setenv("API_SERVER_HOST", tc.host)
			// Invalid port is rejected before binding; this test never opens a listener.
			s := &Server{router: gin.New(), port: -1}
			if err := s.Start(); err == nil {
				t.Fatal("invalid port unexpectedly accepted")
			}
			if s.httpServer.Addr != tc.want {
				t.Fatalf("listener address: got %q want %q", s.httpServer.Addr, tc.want)
			}
		})
	}
}
