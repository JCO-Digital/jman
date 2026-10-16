package api

import (
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/verb"
)

// checkInMessageLimit caps the message a check-in may carry, in runes.
const checkInMessageLimit = 500

// CheckInResponse answers a check-in. Accepted is false when the check is
// paused or orphaned: the check-in was valid but changed nothing.
type CheckInResponse struct {
	OK       bool `json:"ok"`
	Accepted bool `json:"accepted"`
}

// CheckInHandler handles POST /api/checkin and POST /api/checkin/{signal}
// (ok, fail or start; none means ok). It is public: the bearer token, a
// heartbeat check's check-in token, says which check is reporting. An
// optional plain-text body becomes the message of a failure. Invalid
// tokens count towards the client's rate limit, like failed logins.
func CheckInHandler(limiter *LoginRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := limiter.ClientIP(r)
		if !limiter.Allow(ip) {
			WriteError(w, http.StatusTooManyRequests, "Too many invalid check-ins, try again later")
			return
		}

		signal := monitor.Signal(r.PathValue("signal"))
		switch signal {
		case "":
			signal = monitor.SignalOK
		case monitor.SignalOK, monitor.SignalFail, monitor.SignalStart:
		default:
			WriteError(w, http.StatusNotFound, "Unknown check-in signal; use ok, fail or start")
			return
		}

		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		checkID := ""
		if ok {
			var err error
			if checkID, err = db.GetMonitorCheckIDByToken(strings.TrimSpace(token)); err != nil {
				verb.LogPrintf(verb.Normal, "CheckInHandler: %v", err)
				WriteError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
		}
		if checkID == "" {
			limiter.RecordFailure(ip)
			WriteError(w, http.StatusUnauthorized, "Invalid check-in token")
			return
		}
		check, err := db.GetMonitorCheck(checkID)
		if err != nil {
			verb.LogPrintf(verb.Normal, "CheckInHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if check == nil {
			WriteError(w, http.StatusUnauthorized, "Invalid check-in token")
			return
		}

		message := ""
		if r.Body != nil {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 4*checkInMessageLimit))
			message = cleanCheckInMessage(body)
		}

		accepted, err := monitor.CheckIn(*check, signal, message)
		if err != nil {
			verb.LogPrintf(verb.Normal, "CheckInHandler: check %s: %v", check.ID, err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		WriteJSON(w, http.StatusOK, CheckInResponse{OK: true, Accepted: accepted})
	}
}

// cleanCheckInMessage makes a check-in body safe to store and post to
// Slack: valid UTF-8 on one line, without control characters, at most
// checkInMessageLimit runes.
func cleanCheckInMessage(body []byte) string {
	s := strings.ToValidUTF8(string(body), "")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > checkInMessageLimit {
		s = string([]rune(s)[:checkInMessageLimit]) + "…"
	}
	return s
}
