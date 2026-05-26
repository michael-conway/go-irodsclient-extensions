package oidcverify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyTokenSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/drs/protocol/openid-connect/token/introspect":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.Form.Get("token") != "token-1" {
				t.Fatalf("expected introspection token token-1, got %q", r.Form.Get("token"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active":             true,
				"scope":              "openid profile",
				"sub":                "user-123",
				"preferred_username": "from-introspection",
				"client_id":          "irods-go-rest",
				"aud":                []string{"irods-go-rest", "account"},
			})
		case "/realms/drs/protocol/openid-connect/userinfo":
			if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
				t.Fatalf("expected bearer token header, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preferred_username": "test1",
				"sub":                "user-123",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	verifier := NewVerifier(Config{
		BaseURL:      server.URL,
		Realm:        "drs",
		ClientID:     "irods-go-rest",
		ClientSecret: "secret",
	})

	result, err := verifier.VerifyToken(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("verify token failed: %v", err)
	}

	if result.Username != "test1" {
		t.Fatalf("expected trusted username test1, got %q", result.Username)
	}
	if !result.Introspection.Active {
		t.Fatal("expected token to be active")
	}
	if result.Introspection.Subject != "user-123" {
		t.Fatalf("expected subject user-123, got %q", result.Introspection.Subject)
	}
	if len(result.Introspection.Audience) != 2 {
		t.Fatalf("expected 2 audiences, got %v", result.Introspection.Audience)
	}
}

func TestVerifyTokenRejectsInactiveToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/drs/protocol/openid-connect/token/introspect":
			_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
		case "/realms/drs/protocol/openid-connect/userinfo":
			t.Fatal("userinfo should not be called for inactive tokens")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	verifier := NewVerifier(Config{
		BaseURL:  server.URL,
		Realm:    "drs",
		ClientID: "irods-go-rest",
	})

	_, err := verifier.VerifyToken(context.Background(), "token-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestVerifyTokenRejectsMissingPreferredUsername(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/drs/protocol/openid-connect/token/introspect":
			_ = json.NewEncoder(w).Encode(map[string]any{"active": true})
		case "/realms/drs/protocol/openid-connect/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "user-123"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	verifier := NewVerifier(Config{
		BaseURL:  server.URL,
		Realm:    "drs",
		ClientID: "irods-go-rest",
	})

	_, err := verifier.VerifyToken(context.Background(), "token-1")
	if !errors.Is(err, ErrMissingUser) {
		t.Fatalf("expected ErrMissingUser, got %v", err)
	}
}

func TestVerifyTokenReturnsNotConfigured(t *testing.T) {
	verifier := NewVerifier(Config{})
	_, err := verifier.VerifyToken(context.Background(), "token-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
	if !strings.Contains(err.Error(), "oidc_url") {
		t.Fatalf("expected error to include missing config keys, got %q", err.Error())
	}
}
