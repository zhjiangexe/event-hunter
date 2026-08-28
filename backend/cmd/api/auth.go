package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type principal struct {
	Subject string
	Role    string
}

type sessionManager struct{ secret []byte }

const demoSessionTTL = 8 * time.Hour

func (manager sessionManager) requireRead(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := manager.read(request)
		if !ok {
			writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		if !canRead(principal.Role) {
			writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
			return
		}
		next(writer, request)
	}
}

func (manager sessionManager) demoSession(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodDelete {
		http.SetCookie(writer, &http.Cookie{Name: "eh_demo_session", Value: "", MaxAge: -1, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil || !validRole(input.Role) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_ROLE")
		return
	}
	subject := "demo-" + strings.ToLower(input.Role)
	value := manager.sign(subject + "|" + input.Role + "|" + time.Now().Add(demoSessionTTL).Format(time.RFC3339))
	http.SetCookie(writer, &http.Cookie{Name: "eh_demo_session", Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(demoSessionTTL.Seconds())})
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"subject": subject, "role": input.Role, "permissions": permissions(input.Role)})
}

func (manager sessionManager) me(writer http.ResponseWriter, request *http.Request) {
	principal, ok := manager.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	_ = json.NewEncoder(writer).Encode(map[string]any{"subject": principal.Subject, "role": principal.Role, "permissions": permissions(principal.Role)})
}

func (manager sessionManager) read(request *http.Request) (principal, bool) {
	cookie, err := request.Cookie("eh_demo_session")
	if err != nil {
		return principal{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return principal{}, false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 4 || !hmac.Equal([]byte(parts[3]), []byte(manager.signature(strings.Join(parts[:3], "|")))) {
		return principal{}, false
	}
	expires, err := time.Parse(time.RFC3339, parts[2])
	if err != nil || time.Now().After(expires) || !validRole(parts[1]) {
		return principal{}, false
	}
	return principal{Subject: parts[0], Role: parts[1]}, true
}

func (manager sessionManager) sign(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value + "|" + manager.signature(value)))
}
func (manager sessionManager) signature(value string) string {
	sum := hmac.New(sha256.New, manager.secret)
	_, _ = sum.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum.Sum(nil))
}
func validRole(role string) bool {
	return role == "VIEWER" || role == "INVESTIGATOR" || role == "ADMIN"
}
func permissions(role string) []string {
	if role == "VIEWER" {
		return []string{"timeline:read", "investigation:read", "saved_search:write_own"}
	}
	if role == "INVESTIGATOR" {
		return []string{"timeline:read", "investigation:read", "investigation:write", "pattern:execute", "evidence:read", "saved_search:write_own"}
	}
	return []string{"*", "payload:read_sensitive"}
}
func canWrite(role string) bool { return role == "INVESTIGATOR" || role == "ADMIN" }
func canRead(role string) bool  { return validRole(role) }
