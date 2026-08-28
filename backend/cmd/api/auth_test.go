package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionManagerRoundTripAndTamperDetection(t *testing.T) {
	manager := sessionManager{secret: []byte("test-secret")}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/demo-session", strings.NewReader(`{"role":"INVESTIGATOR"}`))
	recorder := httptest.NewRecorder()
	manager.demoSession(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatalf("expected one HttpOnly session cookie, got %#v", cookies)
	}
	if cookies[0].MaxAge != int(demoSessionTTL.Seconds()) {
		t.Fatalf("session MaxAge = %d, want %d", cookies[0].MaxAge, int(demoSessionTTL.Seconds()))
	}
	readRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	readRequest.AddCookie(cookies[0])
	principal, ok := manager.read(readRequest)
	if !ok || principal.Role != "INVESTIGATOR" {
		t.Fatalf("round trip principal = %#v, ok = %v", principal, ok)
	}
	tampered := *cookies[0]
	tamperIndex := len(tampered.Value) / 2
	replacement := byte('A')
	if tampered.Value[tamperIndex] == replacement {
		replacement = 'B'
	}
	tampered.Value = tampered.Value[:tamperIndex] + string(replacement) + tampered.Value[tamperIndex+1:]
	tamperedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	tamperedRequest.AddCookie(&tampered)
	if _, ok := manager.read(tamperedRequest); ok {
		t.Fatal("tampered session was accepted")
	}
}

func TestDemoSessionRejectsUnknownRole(t *testing.T) {
	manager := sessionManager{secret: []byte("test-secret")}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/demo-session", strings.NewReader(`{"role":"SUPERUSER"}`))
	recorder := httptest.NewRecorder()
	manager.demoSession(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
}

func TestPermissionBoundaries(t *testing.T) {
	if canWrite("VIEWER") {
		t.Fatal("viewer unexpectedly has write permission")
	}
	if !canWrite("INVESTIGATOR") || !canWrite("ADMIN") {
		t.Fatal("investigator and admin must have write permission")
	}
	if !canRead("VIEWER") || !canRead("INVESTIGATOR") || !canRead("ADMIN") {
		t.Fatal("all valid roles must have read permission")
	}
}
