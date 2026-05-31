// Package httputil provides shared HTTP helper functions for all AH5 core handlers.
package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// rePascalCase matches a valid AH5 system name: starts with an uppercase letter,
// followed by up to 62 alphanumeric characters.
var rePascalCase = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,62}$`)

// ValidatePascalCase returns an error message if name does not match the PascalCase
// convention (^[A-Z][A-Za-z0-9]{0,62}$). Returns "" if name is valid.
func ValidatePascalCase(name string) string {
	if !rePascalCase.MatchString(name) {
		return fmt.Sprintf("systemName must be PascalCase (^[A-Z][A-Za-z0-9]{0,62}$), got: %q", name)
	}
	return ""
}

// ErrorTypeForStatus maps an HTTP status code to the AH5 exceptionType string.
func ErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_PARAMETER"
	case http.StatusUnauthorized:
		return "AUTH_EXCEPTION"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "DATA_NOT_FOUND"
	case http.StatusLocked:
		return "LOCKED"
	case http.StatusNotImplemented:
		return "NOT_IMPLEMENTED"
	default:
		return "ARROWHEAD_EXCEPTION"
	}
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any, origin string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// WriteError writes an AH5 error envelope as JSON.
func WriteError(w http.ResponseWriter, status int, msg, origin string) {
	WriteJSON(w, status, map[string]any{
		"errorMessage":  msg,
		"errorCode":     status,
		"exceptionType": ErrorTypeForStatus(status),
		"origin":        origin,
	}, origin)
}

// RequireMethod writes 405 and returns false if r.Method != method.
func RequireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		WriteError(w, http.StatusMethodNotAllowed, "method not allowed", "")
		return false
	}
	return true
}

// DecodeJSON decodes the request body into v. Writes 400 and returns false on failure.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "")
		return false
	}
	return true
}

// ExtractBearer returns the Bearer token from the Authorization header, or "".
func ExtractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return after
	}
	return ""
}

// VerifyTokenIdentity checks that the Bearer token in r identifies claimedName.
// When registerAuthURL is empty, returns (true, 0) — open registration mode.
// When set: missing token → (false, 401); auth unreachable → (false, 401) fail-closed;
// name mismatch → (false, 403); name match → (true, 0).
func VerifyTokenIdentity(r *http.Request, registerAuthURL, claimedName string) (bool, int) {
	if registerAuthURL == "" {
		return true, 0
	}
	token := ExtractBearer(r)
	if token == "" {
		return false, http.StatusUnauthorized
	}
	resp, err := http.Get(registerAuthURL + "/authentication/identity/verify/" + token) //nolint:noctx
	if err != nil || resp.StatusCode != http.StatusOK {
		return false, http.StatusUnauthorized
	}
	defer resp.Body.Close() //nolint:errcheck
	var body struct {
		SystemName string `json:"systemName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, http.StatusUnauthorized
	}
	if body.SystemName != claimedName {
		return false, http.StatusForbidden
	}
	return true, 0
}

// RequireManagementAuth checks that the request carries a valid sysop Bearer token
// when mgmtAuthURL is configured. Returns true if access is allowed.
//
// When mgmtAuthURL is empty, always returns true (development/PoC mode).
// When set, the token is verified via GET <mgmtAuthURL>/authentication/identity/verify/<token>.
// A missing or invalid token → 401. A valid non-sysop token → 403. Auth system
// unreachable → 401 (fail-closed).
func RequireManagementAuth(w http.ResponseWriter, r *http.Request, mgmtAuthURL, origin string) bool {
	if mgmtAuthURL == "" {
		return true
	}
	token := ExtractBearer(r)
	if token == "" {
		WriteError(w, http.StatusUnauthorized, "Authorization: Bearer token required for management access", origin)
		return false
	}
	resp, err := http.Get(mgmtAuthURL + "/authentication/identity/verify/" + token) //nolint:noctx
	if err != nil || resp.StatusCode != http.StatusOK {
		WriteError(w, http.StatusUnauthorized, "management access denied: token invalid or auth system unreachable", origin)
		return false
	}
	defer resp.Body.Close() //nolint:errcheck
	var body struct {
		Sysop bool `json:"sysop"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || !body.Sysop {
		WriteError(w, http.StatusForbidden, "management access denied: sysop privilege required", origin)
		return false
	}
	return true
}
