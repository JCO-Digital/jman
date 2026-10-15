package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
)

const testVerifier = "a-pkce-verifier-that-is-long-enough-to-be-realistic-0123456789"

func cliTestUsers() *config.UsersConfig {
	return &config.UsersConfig{
		JWTSecret:                  "a-long-enough-secret-for-cli-token-tests",
		AccessTokenLifetimeMinutes: 15,
		Users: []config.UserEntry{
			{Username: "alice", DisplayName: "Alice", Level: config.LevelEdit},
			{Username: "root", DisplayName: "Root", Level: config.LevelAdmin, TOTPSecret: "MFRGGZDFMZTWQ2LK"},
		},
	}
}

// authorizeCLI approves a CLI login as username through the web session
// endpoint and returns the response.
func authorizeCLI(t *testing.T, claims *AuthClaims, challenge string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(cliAuthorizeRequest{Challenge: challenge, DeviceName: "  laptop\n"})
	req := httptest.NewRequest("POST", "/api/cli/authorize", bytes.NewReader(body)).
		WithContext(contextWithClaims(context.Background(), claims))
	w := httptest.NewRecorder()
	CLIAuthorizeHandler(w, req)
	return w
}

func exchangeCLICode(t *testing.T, usersCfg *config.UsersConfig, code, verifier string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(cliTokenRequest{Code: code, Verifier: verifier})
	req := httptest.NewRequest("POST", "/api/cli/token", bytes.NewReader(body))
	w := httptest.NewRecorder()
	CLITokenHandler(usersCfg, NewLoginRateLimiter(false)).ServeHTTP(w, req)
	return w
}

// loginCLI runs the whole login flow for username and returns the token.
func loginCLI(t *testing.T, usersCfg *config.UsersConfig, username string) string {
	t.Helper()
	w := authorizeCLI(t, &AuthClaims{Username: username}, pkceChallenge(testVerifier))
	if w.Code != http.StatusOK {
		t.Fatalf("authorize: %d %s", w.Code, w.Body.String())
	}
	var auth map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &auth)

	w = exchangeCLICode(t, usersCfg, auth["code"], testVerifier)
	if w.Code != http.StatusOK {
		t.Fatalf("token: %d %s", w.Code, w.Body.String())
	}
	var resp cliTokenResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.DeviceName != "laptop" || resp.User.Username != username {
		t.Fatalf("unexpected token response: %+v", resp)
	}
	return resp.Token
}

// asCLI sends a request with a CLI token through AuthMiddleware and returns
// the claims the handler saw (nil if it wasn't reached).
func asCLI(usersCfg *config.UsersConfig, token string, h http.HandlerFunc) (*httptest.ResponseRecorder, *AuthClaims) {
	var seen *AuthClaims
	handler := AuthMiddleware(usersCfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = GetAuthClaims(r.Context())
		if h != nil {
			h(w, r)
		}
	}))
	req := httptest.NewRequest("GET", "/api/anything", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w, seen
}

func TestCLILoginFlow(t *testing.T) {
	setupSiteManagementTest(t)
	usersCfg := cliTestUsers()

	t.Run("rejects an invalid challenge", func(t *testing.T) {
		if w := authorizeCLI(t, &AuthClaims{Username: "alice"}, "not-a-challenge"); w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("a code works once, and only with its verifier", func(t *testing.T) {
		w := authorizeCLI(t, &AuthClaims{Username: "alice"}, pkceChallenge(testVerifier))
		var auth map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &auth)

		if w := exchangeCLICode(t, usersCfg, auth["code"], "wrong-verifier"); w.Code != http.StatusUnauthorized {
			t.Fatalf("wrong verifier: expected 401, got %d", w.Code)
		}
		if w := exchangeCLICode(t, usersCfg, auth["code"], testVerifier); w.Code != http.StatusUnauthorized {
			t.Fatalf("reused code: expected 401, got %d", w.Code)
		}
	})

	t.Run("an expired code is rejected", func(t *testing.T) {
		w := authorizeCLI(t, &AuthClaims{Username: "alice"}, pkceChallenge(testVerifier))
		var auth map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &auth)
		cliAuthCodes.Lock()
		ac := cliAuthCodes.m[auth["code"]]
		ac.expiresAt = time.Now().Add(-time.Second)
		cliAuthCodes.m[auth["code"]] = ac
		cliAuthCodes.Unlock()

		if w := exchangeCLICode(t, usersCfg, auth["code"], testVerifier); w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("the token authenticates with the user's current level", func(t *testing.T) {
		token := loginCLI(t, usersCfg, "alice")

		w, claims := asCLI(usersCfg, token, nil)
		if w.Code != http.StatusOK || claims == nil {
			t.Fatalf("expected the request through, got %d %s", w.Code, w.Body.String())
		}
		if claims.Username != "alice" || claims.Level != config.LevelEdit || claims.CLITokenID == 0 {
			t.Errorf("unexpected claims: %+v", claims)
		}

		// A role change applies to the existing token at once.
		usersCfg.Users[0].Level = config.LevelBasic
		if _, claims := asCLI(usersCfg, token, nil); claims == nil || claims.Level != config.LevelBasic {
			t.Errorf("expected level basic after demotion, got %+v", claims)
		}
		usersCfg.Users[0].Level = config.LevelEdit

		// A CLI token can't approve another CLI login.
		if w := authorizeCLI(t, claims, pkceChallenge(testVerifier)); w.Code != http.StatusForbidden {
			t.Errorf("authorize with a CLI token: expected 403, got %d", w.Code)
		}

		// A credential change revokes it.
		usersCfg.Users[0].TokenVersion++
		if w, _ := asCLI(usersCfg, token, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("after a credential change: expected 401, got %d", w.Code)
		}
		usersCfg.Users[0].TokenVersion--
	})

	t.Run("an admin without 2FA gets the downgraded level", func(t *testing.T) {
		usersCfg.Users[1].TOTPSecret = ""
		defer func() { usersCfg.Users[1].TOTPSecret = "MFRGGZDFMZTWQ2LK" }()
		token := loginCLI(t, usersCfg, "root")
		if _, claims := asCLI(usersCfg, token, nil); claims == nil || claims.Level != config.LevelEdit {
			t.Errorf("expected level edit, got %+v", claims)
		}
	})
}

func TestCLITokenIdleExpiry(t *testing.T) {
	setupSiteManagementTest(t)
	usersCfg := cliTestUsers()
	token := loginCLI(t, usersCfg, "alice")

	// Used recently enough: still valid, and use doesn't rewrite last_used_at
	// more than once an hour.
	old := time.Now().UTC().Add(-db.CLITokenIdleLifetime + time.Hour)
	if _, err := db.GetAPIDB().Exec(`UPDATE cli_tokens SET last_used_at = ?`, old); err != nil {
		t.Fatal(err)
	}
	if w, _ := asCLI(usersCfg, token, nil); w.Code != http.StatusOK {
		t.Fatalf("expected a token used 89 days ago to work, got %d", w.Code)
	}
	tokens, _ := db.ListCLITokens("alice")
	if len(tokens) != 1 || time.Since(tokens[0].LastUsedAt) > time.Minute {
		t.Fatalf("expected use to extend the token, got %+v", tokens)
	}

	// Unused for the whole lifetime: expired.
	expired := time.Now().UTC().Add(-db.CLITokenIdleLifetime - time.Minute)
	if _, err := db.GetAPIDB().Exec(`UPDATE cli_tokens SET last_used_at = ?`, expired); err != nil {
		t.Fatal(err)
	}
	if w, _ := asCLI(usersCfg, token, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("expected an idle token to be rejected, got %d", w.Code)
	}
	if tokens, _ := db.ListCLITokens("alice"); len(tokens) != 0 {
		t.Errorf("expected expired tokens to be hidden, got %+v", tokens)
	}
}

func TestCLITokenListAndRevoke(t *testing.T) {
	setupSiteManagementTest(t)
	usersCfg := cliTestUsers()
	aliceToken := loginCLI(t, usersCfg, "alice")
	rootToken := loginCLI(t, usersCfg, "root")

	list := func(token, query string) (int, []apiCLIToken) {
		w, _ := asCLI(usersCfg, token, func(w http.ResponseWriter, r *http.Request) {
			r.URL.RawQuery = query
			ListCLITokensHandler(w, r)
		})
		var out []apiCLIToken
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	del := func(token, id string) int {
		w, _ := asCLI(usersCfg, token, func(w http.ResponseWriter, r *http.Request) {
			r.SetPathValue("id", id)
			DeleteCLITokenHandler(w, r)
		})
		return w.Code
	}

	code, own := list(aliceToken, "")
	if code != http.StatusOK || len(own) != 1 || own[0].Username != "alice" || !own[0].Current {
		t.Fatalf("alice's list: %d %+v", code, own)
	}
	if code, _ := list(aliceToken, "all=true"); code != http.StatusForbidden {
		t.Errorf("non-admin listing all: expected 403, got %d", code)
	}
	_, all := list(rootToken, "all=true")
	if len(all) != 2 {
		t.Fatalf("admin listing all: expected 2 tokens, got %+v", all)
	}

	var rootID int64
	for _, tok := range all {
		if tok.Username == "root" {
			rootID = tok.ID
		}
	}
	if code := del(aliceToken, jsonID(rootID)); code != http.StatusNotFound {
		t.Errorf("alice revoking root's token: expected 404, got %d", code)
	}
	if code := del(rootToken, jsonID(own[0].ID)); code != http.StatusNoContent {
		t.Errorf("admin revoking alice's token: expected 204, got %d", code)
	}
	if w, _ := asCLI(usersCfg, aliceToken, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("revoked token: expected 401, got %d", w.Code)
	}

	// jman logout revokes the token it is called with.
	w, _ := asCLI(usersCfg, rootToken, RevokeCurrentCLITokenHandler)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", w.Code)
	}
	if w, _ := asCLI(usersCfg, rootToken, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("after logout: expected 401, got %d", w.Code)
	}
}

func jsonID(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}
