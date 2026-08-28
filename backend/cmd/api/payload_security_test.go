package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaskPayloadRecursively(t *testing.T) {
	payload := map[string]any{
		"orderId": "ORDER-1",
		"customer": map[string]any{
			"customer_id": "CUSTOMER-1",
			"credentials": map[string]any{"access_token": "token-value", "clientSecret": "secret-value"},
		},
		"recipients": []any{
			map[string]any{"emailAddress": "buyer@example.com", "phone_number": "+886900000000"},
		},
		"charges": []any{map[string]any{"amount": 1280.0}},
	}

	masked := maskPayload(payload)
	customer := masked["customer"].(map[string]any)
	credentials := customer["credentials"].(map[string]any)
	recipient := masked["recipients"].([]any)[0].(map[string]any)
	charge := masked["charges"].([]any)[0].(map[string]any)

	if !strings.HasPrefix(customer["customer_id"].(string), "CUSTOMER-***-") {
		t.Fatalf("nested customer ID was not tokenized: %#v", masked)
	}
	if credentials["access_token"] != "[REDACTED]" || credentials["clientSecret"] != "[REDACTED]" {
		t.Fatalf("nested credentials were not redacted: %#v", masked)
	}
	if recipient["emailAddress"] != "[REDACTED_PII]" || recipient["phone_number"] != "[REDACTED_PII]" {
		t.Fatalf("array PII was not redacted: %#v", masked)
	}
	if charge["amount"] != "[REDACTED_AMOUNT]" {
		t.Fatalf("array amount was not redacted: %#v", masked)
	}
	if payload["customer"].(map[string]any)["customer_id"] != "CUSTOMER-1" {
		t.Fatal("masking mutated the source payload")
	}
}

func TestAuthorizePayloadRequestRequiresAdmin(t *testing.T) {
	manager := sessionManager{secret: []byte("test-secret")}
	api := timelineAPI{sessions: &manager}

	for _, role := range []string{"VIEWER", "INVESTIGATOR", "ADMIN"} {
		t.Run(role, func(t *testing.T) {
			sessionRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/demo-session", strings.NewReader(`{"role":"`+role+`"}`))
			sessionResponse := httptest.NewRecorder()
			manager.demoSession(sessionResponse, sessionRequest)

			request := httptest.NewRequest(http.MethodGet, "/api/v1/events/search?include_payload=true", nil)
			request.AddCookie(sessionResponse.Result().Cookies()[0])
			response := httptest.NewRecorder()
			includePayload, authorized := api.authorizePayloadRequest(response, request)

			if role == "ADMIN" {
				if !authorized || !includePayload || response.Code != http.StatusOK {
					t.Fatalf("ADMIN authorization = (%v, %v), status = %d", includePayload, authorized, response.Code)
				}
				return
			}
			if authorized || includePayload || response.Code != http.StatusForbidden {
				t.Fatalf("%s authorization = (%v, %v), status = %d", role, includePayload, authorized, response.Code)
			}
		})
	}
}
