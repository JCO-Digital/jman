package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// The jman CLI logs in through the web UI with an OAuth-style loopback
// redirect and PKCE:
//
//  1. The CLI opens <webUrl>/cli-login with a PKCE challenge, its device
//     name and the port of a listener on 127.0.0.1.
//  2. The logged-in user approves it, and the web UI calls
//     POST /api/cli/authorize, which returns a short-lived one-time code.
//  3. The web UI redirects the browser to the CLI's listener with the code.
//  4. The CLI sends the code and its PKCE verifier to POST /api/cli/token
//     and gets a long-lived CLI token (see db.CreateCLIToken).
//
// The PKCE check means a code intercepted on its way to the CLI is useless
// without the verifier, which never leaves the CLI.

// cliAuthCodeLifetime is how long an approved login code can be exchanged.
const cliAuthCodeLifetime = 2 * time.Minute

// maxDeviceNameLength bounds the device name a CLI token is labelled with.
const maxDeviceNameLength = 100

type cliAuthCode struct {
	username   string
	deviceName string
	challenge  string
	expiresAt  time.Time
}

// cliAuthCodes holds approved, not yet exchanged login codes. They live
// for minutes, so keeping them in memory is enough: a restart only fails
// logins in progress.
var cliAuthCodes = struct {
	sync.Mutex
	m map[string]cliAuthCode
}{m: map[string]cliAuthCode{}}

type cliAuthorizeRequest struct {
	Challenge  string `json:"challenge"`
	DeviceName string `json:"device_name"`
}

type cliTokenRequest struct {
	Code     string `json:"code"`
	Verifier string `json:"verifier"`
}

type cliTokenResponse struct {
	Token      string        `json:"token"`
	DeviceName string        `json:"device_name"`
	ExpiresAt  time.Time     `json:"expires_at"`
	User       loginRespUser `json:"user"`
}

// apiCLIToken is a CLI token in GET /api/cli/tokens. Current marks the
// token the request was made with.
type apiCLIToken struct {
	models.CLIToken
	Current bool `json:"current"`
}

// isPKCEChallenge reports whether s looks like an S256 PKCE challenge: the
// unpadded base64url encoding of a SHA-256 hash.
func isPKCEChallenge(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

// pkceChallenge returns the S256 PKCE challenge of verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// cleanDeviceName trims a device name and drops control characters.
func cleanDeviceName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(name))
	if r := []rune(name); len(r) > maxDeviceNameLength {
		name = string(r[:maxDeviceNameLength])
	}
	return name
}

// CLIAuthorizeHandler approves a CLI login for the logged-in web user and
// returns the one-time code the web UI hands to the CLI. Only a web
// session can approve a login, not a CLI token.
//
//	POST /api/cli/authorize
//	{"challenge": "<S256 PKCE challenge>", "device_name": "laptop"}
func CLIAuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	if claims.CLITokenID != 0 {
		WriteError(w, http.StatusForbidden, "CLI logins must be approved in the web UI")
		return
	}

	var req cliAuthorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !isPKCEChallenge(req.Challenge) {
		WriteError(w, http.StatusBadRequest, "Invalid challenge")
		return
	}
	deviceName := cleanDeviceName(req.DeviceName)
	if deviceName == "" {
		WriteError(w, http.StatusBadRequest, "Device name is required")
		return
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		verb.LogPrintf(verb.Normal, "CLIAuthorizeHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	code := base64.RawURLEncoding.EncodeToString(b)

	now := time.Now()
	cliAuthCodes.Lock()
	for c, ac := range cliAuthCodes.m {
		if now.After(ac.expiresAt) {
			delete(cliAuthCodes.m, c)
		}
	}
	cliAuthCodes.m[code] = cliAuthCode{
		username:   claims.Username,
		deviceName: deviceName,
		challenge:  req.Challenge,
		expiresAt:  now.Add(cliAuthCodeLifetime),
	}
	cliAuthCodes.Unlock()

	WriteJSON(w, http.StatusOK, map[string]string{"code": code})
}

// CLITokenHandler exchanges an approved login code and its PKCE verifier
// for a CLI token. It is a public route: the code and verifier are the
// credentials. A code works once, whether or not the exchange succeeds.
//
//	POST /api/cli/token
//	{"code": "...", "verifier": "..."}
func CLITokenHandler(usersCfg *config.UsersConfig, limiter *LoginRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP := limiter.ClientIP(r)
		if !limiter.Allow(clientIP) {
			WriteError(w, http.StatusTooManyRequests, "Too many login attempts, try again later")
			return
		}

		var req cliTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.Verifier == "" {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		cliAuthCodes.Lock()
		ac, ok := cliAuthCodes.m[req.Code]
		delete(cliAuthCodes.m, req.Code)
		cliAuthCodes.Unlock()

		if !ok || time.Now().After(ac.expiresAt) ||
			subtle.ConstantTimeCompare([]byte(pkceChallenge(req.Verifier)), []byte(ac.challenge)) != 1 {
			limiter.RecordFailure(clientIP)
			WriteError(w, http.StatusUnauthorized, "Invalid or expired login code")
			return
		}

		usersCfg.LockRead()
		user := config.FindUser(usersCfg, ac.username)
		var tokenVersion int
		var respUser loginRespUser
		if user != nil {
			tokenVersion = user.TokenVersion
			respUser = newLoginRespUser(user)
		}
		usersCfg.UnlockRead()
		if user == nil {
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if err := db.DeleteExpiredCLITokens(); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
		t, raw, err := db.CreateCLIToken(ac.username, ac.deviceName, tokenVersion)
		if err != nil {
			verb.LogPrintf(verb.Normal, "CLITokenHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		limiter.Reset(clientIP)
		verb.LogPrintf(verb.Normal, "Issued CLI token %d to %s for %q", t.ID, t.Username, t.DeviceName)

		WriteJSON(w, http.StatusOK, cliTokenResponse{
			Token:      raw,
			DeviceName: t.DeviceName,
			ExpiresAt:  t.ExpiresAt,
			User:       respUser,
		})
	}
}

// ListCLITokensHandler returns the user's CLI tokens. Admins can pass
// ?all=true to list every user's tokens.
//
//	GET /api/cli/tokens
func ListCLITokensHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	username := claims.Username
	if r.URL.Query().Get("all") == "true" {
		if levelToInt(claims.Level) < levelToInt(config.LevelAdmin) {
			WriteError(w, http.StatusForbidden, "Insufficient permissions")
			return
		}
		username = ""
	}

	tokens, err := db.ListCLITokens(username)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListCLITokensHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	out := make([]apiCLIToken, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, apiCLIToken{CLIToken: t, Current: t.ID == claims.CLITokenID})
	}
	WriteJSON(w, http.StatusOK, out)
}

// DeleteCLITokenHandler revokes one of the user's CLI tokens. Admins can
// revoke anyone's.
//
//	DELETE /api/cli/tokens/{id}
func DeleteCLITokenHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}
	username := claims.Username
	if levelToInt(claims.Level) >= levelToInt(config.LevelAdmin) {
		username = ""
	}
	deleteCLIToken(w, id, username, claims.Username)
}

// RevokeCurrentCLITokenHandler revokes the CLI token the request was made
// with. `jman logout` uses it.
//
//	DELETE /api/cli/token
func RevokeCurrentCLITokenHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	if claims.CLITokenID == 0 {
		WriteError(w, http.StatusBadRequest, "Request was not made with a CLI token")
		return
	}
	deleteCLIToken(w, claims.CLITokenID, claims.Username, claims.Username)
}

func deleteCLIToken(w http.ResponseWriter, id int64, owner, actor string) {
	deleted, err := db.DeleteCLIToken(id, owner)
	if err != nil {
		verb.LogPrintf(verb.Normal, "deleteCLIToken: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !deleted {
		WriteError(w, http.StatusNotFound, "CLI token not found")
		return
	}
	verb.LogPrintf(verb.Normal, "CLI token %d revoked by %s", id, actor)
	w.WriteHeader(http.StatusNoContent)
}
