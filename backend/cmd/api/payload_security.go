package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

func canReadSensitivePayload(role string) bool {
	return role == "ADMIN"
}

func (api timelineAPI) authorizePayloadRequest(writer http.ResponseWriter, request *http.Request) (bool, bool) {
	if request.URL.Query().Get("include_payload") != "true" {
		return false, true
	}
	if api.sessions == nil {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return false, false
	}
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return false, false
	}
	if !canReadSensitivePayload(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return false, false
	}
	return true, true
}

func maskPayload(payload map[string]any) map[string]any {
	return maskPayloadValue("", payload).(map[string]any)
}

func maskPayloadValue(field string, value any) any {
	switch normalizedSensitiveField(field) {
	case "customerid":
		sum := sha256.Sum256([]byte(fmt.Sprint(value)))
		return fmt.Sprintf("CUSTOMER-***-%x", sum[:4])
	case "totalamount", "amount":
		return "[REDACTED_AMOUNT]"
	case "email", "emailaddress", "phone", "phonenumber", "mobile", "mobilephone":
		return "[REDACTED_PII]"
	case "reason", "cardnumber", "cvv", "password", "passphrase", "secret", "clientsecret", "apikey", "token", "accesstoken", "refreshtoken", "authorization", "cookie", "sessioncookie":
		return "[REDACTED]"
	}

	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, child := range typed {
			masked[key] = maskPayloadValue(key, child)
		}
		return masked
	case []any:
		masked := make([]any, len(typed))
		for index, child := range typed {
			masked[index] = maskPayloadValue("", child)
		}
		return masked
	default:
		return value
	}
}

func normalizedSensitiveField(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return unicode.ToLower(char)
		}
		return -1
	}, value)
}
