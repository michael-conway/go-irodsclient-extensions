package oidcverify

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNotConfigured = errors.New("oidc verifier is not configured")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrMissingUser   = errors.New("authoritative user identity is missing")
)

type Config struct {
	BaseURL            string
	Realm              string
	ClientID           string
	ClientSecret       string
	InsecureSkipVerify bool
	HTTPClient         *http.Client
}

type Verifier struct {
	httpClient   *http.Client
	baseURL      string
	realm        string
	clientID     string
	clientSecret string
}

type VerifiedToken struct {
	Introspection Introspection
	UserInfo      UserInfo
	Username      string
}

type Introspection struct {
	Active            bool
	Scope             string
	PreferredUsername string
	Subject           string
	ClientID          string
	Audience          []string
}

type UserInfo struct {
	PreferredUsername string
	Subject           string
}

func NewVerifier(config Config) *Verifier {
	httpClient := config.HTTPClient
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
		}
		httpClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		}
	}

	return &Verifier{
		httpClient:   httpClient,
		baseURL:      strings.TrimRight(config.BaseURL, "/"),
		realm:        strings.TrimSpace(config.Realm),
		clientID:     strings.TrimSpace(config.ClientID),
		clientSecret: config.ClientSecret,
	}
}

func (v *Verifier) VerifyToken(ctx context.Context, accessToken string) (*VerifiedToken, error) {
	if err := v.configError(); err != nil {
		return nil, err
	}

	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, ErrUnauthorized
	}

	introspection, err := v.introspectToken(ctx, token)
	if err != nil {
		return nil, err
	}

	if !introspection.Active {
		return nil, ErrUnauthorized
	}

	userInfo, err := v.userInfo(ctx, token)
	if err != nil {
		return nil, err
	}

	username := strings.TrimSpace(userInfo.PreferredUsername)
	if username == "" {
		return nil, ErrMissingUser
	}

	return &VerifiedToken{
		Introspection: introspection,
		UserInfo:      userInfo,
		Username:      username,
	}, nil
}

func (v *Verifier) configError() error {
	if v == nil {
		return ErrNotConfigured
	}

	missing := make([]string, 0, 3)
	if strings.TrimSpace(v.baseURL) == "" {
		missing = append(missing, "oidc_url")
	}
	if strings.TrimSpace(v.realm) == "" {
		missing = append(missing, "oidc_realm")
	}
	if strings.TrimSpace(v.clientID) == "" {
		missing = append(missing, "oidc_client_id")
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: missing %s", ErrNotConfigured, strings.Join(missing, ", "))
}

func (v *Verifier) introspectToken(ctx context.Context, accessToken string) (Introspection, error) {
	form := url.Values{}
	form.Set("token", accessToken)
	form.Set("client_id", v.clientID)
	if strings.TrimSpace(v.clientSecret) != "" {
		form.Set("client_secret", v.clientSecret)
	}

	introspectURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token/introspect", v.baseURL, url.PathEscape(v.realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, introspectURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Introspection{}, fmt.Errorf("build keycloak introspection request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return Introspection{}, fmt.Errorf("request keycloak introspection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return Introspection{}, ErrUnauthorized
		}
		return Introspection{}, fmt.Errorf("keycloak introspection failed: %s", resp.Status)
	}

	var payload struct {
		Active            bool   `json:"active"`
		Scope             string `json:"scope"`
		PreferredUsername string `json:"preferred_username"`
		Sub               string `json:"sub"`
		ClientID          string `json:"client_id"`
		Audience          any    `json:"aud"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Introspection{}, fmt.Errorf("decode keycloak introspection response: %w", err)
	}

	return Introspection{
		Active:            payload.Active,
		Scope:             payload.Scope,
		PreferredUsername: payload.PreferredUsername,
		Subject:           payload.Sub,
		ClientID:          strings.TrimSpace(payload.ClientID),
		Audience:          audienceValues(payload.Audience),
	}, nil
}

func (v *Verifier) userInfo(ctx context.Context, accessToken string) (UserInfo, error) {
	userInfoURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/userinfo", v.baseURL, url.PathEscape(v.realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return UserInfo{}, fmt.Errorf("build keycloak userinfo request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return UserInfo{}, fmt.Errorf("request keycloak userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return UserInfo{}, ErrUnauthorized
		}
		return UserInfo{}, fmt.Errorf("keycloak userinfo failed: %s", resp.Status)
	}

	var payload struct {
		PreferredUsername string `json:"preferred_username"`
		Sub               string `json:"sub"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return UserInfo{}, fmt.Errorf("decode keycloak userinfo response: %w", err)
	}

	return UserInfo{
		PreferredUsername: strings.TrimSpace(payload.PreferredUsername),
		Subject:           strings.TrimSpace(payload.Sub),
	}, nil
}

func audienceValues(raw any) []string {
	switch typed := raw.(type) {
	case string:
		audience := strings.TrimSpace(typed)
		if audience == "" {
			return nil
		}
		return []string{audience}
	case []any:
		values := make([]string, 0, len(typed))
		for _, value := range typed {
			if audience, ok := value.(string); ok {
				audience = strings.TrimSpace(audience)
				if audience != "" {
					values = append(values, audience)
				}
			}
		}
		return values
	default:
		return nil
	}
}
