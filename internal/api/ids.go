package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/utils"
)

// resolveSiteUUID parses a site identifier that may be either a UUID or a
// legacy SpinupWP integer ID, and returns the canonical site UUID.
func resolveSiteUUID(rawID string) (string, error) {
	return resolveUUID(rawID, utils.SpinupWPSiteUUID, "site")
}

// resolveServerUUID parses a server identifier that may be either a UUID or
// a legacy SpinupWP integer ID, and returns the canonical server UUID.
func resolveServerUUID(rawID string) (string, error) {
	return resolveUUID(rawID, utils.SpinupWPServerUUID, "server")
}

func resolveUUID(rawID string, fromLegacy func(int) string, kind string) (string, error) {
	rawID = strings.TrimSpace(rawID)
	if utils.IsValidUUID(rawID) {
		return strings.ToLower(rawID), nil
	}
	if num, err := strconv.Atoi(rawID); err == nil && num > 0 {
		return fromLegacy(num), nil
	}
	return "", fmt.Errorf("invalid %s ID: %q", kind, rawID)
}

// FlexID is a request-body identifier that accepts a JSON string (a UUID) or,
// for compatibility with older clients, a JSON integer (a legacy SpinupWP
// ID). null or "" decode to the empty FlexID, meaning "no link". Callers
// normalise it with resolveSiteUUID / resolveServerUUID.
type FlexID string

// UnmarshalJSON implements json.Unmarshaler.
func (f *FlexID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*f = ""
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*f = FlexID(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("identifier must be a string or integer")
	}
	id, err := strconv.Atoi(n.String())
	if err != nil {
		return fmt.Errorf("identifier must be a string or integer, got %s", n)
	}
	*f = FlexID(strconv.Itoa(id))
	return nil
}

// optionalSiteUUID resolves an optional request-body site link: nil when
// absent/empty, the canonical UUID otherwise.
func optionalSiteUUID(id *FlexID) (*string, error) {
	return optionalUUID(id, resolveSiteUUID)
}

// optionalServerUUID resolves an optional request-body server link: nil when
// absent/empty, the canonical UUID otherwise.
func optionalServerUUID(id *FlexID) (*string, error) {
	return optionalUUID(id, resolveServerUUID)
}

func optionalUUID(id *FlexID, resolve func(string) (string, error)) (*string, error) {
	if id == nil || *id == "" {
		return nil, nil
	}
	resolved, err := resolve(string(*id))
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}
