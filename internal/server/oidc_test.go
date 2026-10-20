package server

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/config"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// fakeIdP is a tiny OpenID provider: it signs whoever `email` says.
type fakeIdP struct {
	*httptest.Server
	key, forger *rsa.PrivateKey
	clientID    string
	email       string
	verified    bool
	forge       bool
	nonces      map[string]string // code → nonce
	challenges  map[string]string // code → PKCE challenge
}

func newFakeIdP(t *testing.T, clientID string) *fakeIdP {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	f, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := &fakeIdP{key: k, forger: f, clientID: clientID, verified: true, nonces: map[string]string{}, challenges: map[string]string{}}
	mux := http.NewServeMux()
	p.Server = httptest.NewServer(mux)
	b64 := base64.RawURLEncoding.EncodeToString
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": p.URL, "authorization_endpoint": p.URL + "/authorize", "token_endpoint": p.URL + "/token", "jwks_uri": p.URL + "/jwks"})
	})
	mux.HandleFunc("/api/v1/auth/subject", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "iss": p.URL})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "use": "sig", "kid": "k1", "n": b64(k.N.Bytes()), "e": b64(big.NewInt(int64(k.E)).Bytes())}}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != p.clientID || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "bad request", 400)
			return
		}
		code := store.RandomToken(8)
		p.nonces[code], p.challenges[code] = q.Get("nonce"), q.Get("code_challenge")
		http.Redirect(w, r, q.Get("redirect_uri")+"?"+url.Values{"code": {code}, "state": {q.Get("state")}}.Encode(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		id, secret, _ := r.BasicAuth()
		code := r.FormValue("code")
		sum := sha256.Sum256([]byte(r.FormValue("code_verifier")))
		if id != p.clientID || secret != "shh" || p.challenges[code] != b64(sum[:]) {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		head, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
		body, _ := json.Marshal(map[string]any{"iss": p.URL, "sub": strings.ToLower(p.email), "auth_revision": int64(1), "aud": p.clientID, "exp": time.Now().Add(time.Hour).Unix(), "nonce": p.nonces[code], "email": p.email, "email_verified": p.verified, "name": "Aleksandar"})
		signing := b64(head) + "." + b64(body)
		h := sha256.Sum256([]byte(signing))
		signer := p.key
		if p.forge {
			signer = p.forger
		}
		sig, _ := rsa.SignPKCS1v15(rand.Reader, signer, crypto.SHA256, h[:])
		json.NewEncoder(w).Encode(map[string]string{"id_token": signing + "." + b64(sig), "access_token": "at", "token_type": "Bearer"})
	})
	t.Cleanup(p.Close)
	return p
}

func testConfig(dir string) *config.Config {
	return &config.Config{DataDir: dir, DBPath: dir + "/t.db", FilesDir: dir + "/files", AuthMode: "local", AuthProxyHeader: "Remote-User", AuthProxyEmailHeader: "Remote-Email",
		DefaultCurrency: "EUR", DistanceUnit: "km", VolumeUnit: "l", FXProvider: "none", MaxUploadBytes: 1 << 20, NotifyHour: 8, UserAgent: "test",
		SessionTTL: time.Hour, OIDCName: "single sign-on"}
}

// browser drives the sign-in dance against the app and reports where it ended up.
type browser struct {
	t  *testing.T
	ts *httptest.Server
}

func (b browser) signIn(next string) (*url.URL, *http.Client) {
	jar, _ := cookiejar.New(nil)
	var last *url.URL
	cl := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		last = req.URL
		if strings.HasPrefix(req.URL.String(), b.ts.URL) && !strings.HasPrefix(req.URL.Path, "/api/") {
			return http.ErrUseLastResponse // back in the app
		}
		return nil
	}}
	res, err := cl.Get(b.ts.URL + "/api/v1/auth/oidc/start?next=" + url.QueryEscape(next))
	if err != nil {
		b.t.Fatal(err)
	}
	res.Body.Close()
	return last, cl
}

func (b browser) me(cl *http.Client) int {
	res, err := cl.Get(b.ts.URL + "/api/v1/auth/me")
	if err != nil {
		b.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func (b browser) status() map[string]any {
	var status map[string]any
	res, err := http.Get(b.ts.URL + "/api/v1/auth/status")
	if err != nil {
		b.t.Fatal(err)
	}
	json.NewDecoder(res.Body).Decode(&status)
	res.Body.Close()
	return status
}

// TestOIDCSignIn covers the relying party with explicit OIDC_* settings.
func TestOIDCSignIn(t *testing.T) {
	idp := newFakeIdP(t, "garage")
	dir := t.TempDir()
	cfg := testConfig(dir)
	cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCName, cfg.OIDCAutoCreate = idp.URL, "garage", "shh", "PocketID", false
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewUnstartedServer(nil)
	cfg.BaseURL = "http://" + ts.Listener.Addr().String()
	s := New(cfg, st, log, nil)
	if err := s.core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = s.Router()
	ts.Start()
	t.Cleanup(ts.Close)
	b := browser{t, ts}
	ctx := context.Background()
	_ = st.CreateUser(ctx, &store.User{Email: "aleksandar@example.com", Name: "A", Role: "admin", PasswordHash: "x"})

	idp.email = "Aleksandar@Example.com"
	end, cl := b.signIn("/vehicles")
	if end == nil || end.Path != "/vehicles" || b.me(cl) != 200 {
		t.Fatalf("happy path ended at %v, me=%d", end, b.me(cl))
	}

	// the login page learns about the button from the status
	if status := b.status(); status["oidc"] != true || status["oidc_name"] != "PocketID" {
		t.Errorf("status %v", status)
	}

	for _, c := range []struct {
		name, email string
		verified    bool
		forge       bool
		next, want  string
	}{
		{"unknown email", "stranger@example.com", true, false, "/", "No Pocket Garage account"},
		{"unverified email", "aleksandar@example.com", false, false, "/", "verified email"},
		{"forged signature", "aleksandar@example.com", true, true, "/", "did not check out"},
	} {
		idp.email, idp.verified, idp.forge = c.email, c.verified, c.forge
		end, cl := b.signIn(c.next)
		if end == nil || end.Path != "/login" || !strings.Contains(end.Query().Get("error"), c.want) || b.me(cl) != 401 {
			t.Errorf("%s: ended at %v", c.name, end)
		}
	}

	// the return path cannot leave the site
	idp.email, idp.verified, idp.forge = "aleksandar@example.com", true, false
	if end, _ := b.signIn("//evil.example/x"); end == nil || end.Host != strings.TrimPrefix(ts.URL, "http://") || end.Path != "/" {
		t.Errorf("open redirect: %v", end)
	}
	// a callback without the browser's cookie is refused
	res, _ := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Get(ts.URL + "/api/v1/auth/oidc/callback?code=x&state=y")
	if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, "/login?error=") {
		t.Errorf("cookieless callback → %q", loc)
	}
	// password login is still there
	res, _ = http.Post(ts.URL+"/api/v1/auth/login", "application/json", strings.NewReader(`{"email":"aleksandar@example.com","password":"x"}`))
	if res.StatusCode != 401 {
		t.Errorf("password login answered %d, want 401 (wrong password, not switched off)", res.StatusCode)
	}
}

// TestOIDCFromCore covers the zero-configuration path: no OIDC_* settings,
// the client Core issued at registration is on disk, explicit settings win
// when present, and nothing is on without either.
func TestOIDCFromCore(t *testing.T) {
	idp := newFakeIdP(t, "pocket-garage")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// nothing configured: no button, the routes answer 404
	{
		dir := t.TempDir()
		cfg := testConfig(dir)
		st, err := store.Open(cfg.DBPath, log)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		s := New(cfg, st, log, nil)
		if err := s.core.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if s.oidcEnabled() {
			t.Fatal("oidc enabled without settings or Core")
		}
		ts := httptest.NewServer(s.Router())
		t.Cleanup(ts.Close)
		b := browser{t, ts}
		if status := b.status(); status["oidc"] != false || status["oidc_name"] != "single sign-on" {
			t.Errorf("status without oidc: %v", status)
		}
		res, _ := http.Get(ts.URL + "/api/v1/auth/oidc/start")
		if res.StatusCode != 404 || !strings.Contains(res.Header.Get("Content-Type"), "json") {
			t.Errorf("start without oidc: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
		}
		res.Body.Close()
	}

	// Core issued a client: the SDK persisted it next to the app token
	dir := t.TempDir()
	cfg := testConfig(dir)
	cfg.CoreURL = "http://core.invalid"
	if err := os.WriteFile(dir+"/core.token.oidc.json", []byte(`{"issuer":"`+idp.URL+`/","client_id":"pocket-garage","client_secret":"shh"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewUnstartedServer(nil)
	cfg.BaseURL = "http://" + ts.Listener.Addr().String()
	s := New(cfg, st, log, nil) // no Bootstrap: the registration loop would try to reach core.invalid
	ts.Config.Handler = s.Router()
	ts.Start()
	t.Cleanup(ts.Close)
	b := browser{t, ts}

	issuer, clientID, secret, name := s.oidcSettings()
	if issuer != idp.URL || clientID != "pocket-garage" || secret != "shh" || name != "Pocket Core" || !s.oidcAutoCreate() {
		t.Fatalf("settings from Core: %q %q %q %q auto=%v", issuer, clientID, secret, name, s.oidcAutoCreate())
	}
	if status := b.status(); status["oidc"] != true || status["oidc_name"] != "Pocket Core" {
		t.Errorf("status from Core: %v", status)
	}

	// Core vouches for the person: an unknown email gets an account (the first one is admin)
	idp.email = "new@example.com"
	end, cl := b.signIn("/vehicles")
	if end == nil || end.Path != "/vehicles" || b.me(cl) != 200 {
		t.Fatalf("sign-in with the Core-issued client ended at %v, me=%d", end, b.me(cl))
	}
	if u, err := st.GetUserByEmail(context.Background(), "new@example.com"); err != nil || u.Role != "admin" || u.PasswordHash != "" {
		t.Errorf("auto-created user: %+v %v", u, err)
	}

	// explicit settings win over Core's, and the provider cache follows the issuer
	cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCName, cfg.OIDCAutoCreate = "https://sso.example", "env-client", "env-secret", "Env SSO", false
	if issuer, clientID, secret, name := s.oidcSettings(); issuer != "https://sso.example" || clientID != "env-client" || secret != "env-secret" || name != "Env SSO" || s.oidcAutoCreate() {
		t.Errorf("env should win: %q %q %q %q auto=%v", issuer, clientID, secret, name, s.oidcAutoCreate())
	}
	if p := s.provider("https://sso.example"); p.issuerURL != "https://sso.example" || p.disc != nil {
		t.Errorf("provider cache not rebuilt for the new issuer: %+v", p)
	}
}
