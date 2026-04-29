package searchplugin

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrBearerTokenRequired = errors.New("bearer token is required")
	ErrBasicAuthRequired   = errors.New("basic auth username and password are required")
	ErrUnsupportedAuthType = errors.New("unsupported auth type for default endpoint authorizer")
)

type RequestAuthorizer interface {
	Authorize(request *http.Request, endpoint Endpoint, authContext RequestAuthContext) error
}

type RequestAuthorizerFunc func(request *http.Request, endpoint Endpoint, authContext RequestAuthContext) error

func (authorizer RequestAuthorizerFunc) Authorize(request *http.Request, endpoint Endpoint, authContext RequestAuthContext) error {
	return authorizer(request, endpoint, authContext)
}

func NoAuthAuthorizer() RequestAuthorizer {
	return RequestAuthorizerFunc(func(_ *http.Request, _ Endpoint, _ RequestAuthContext) error {
		return nil
	})
}

func PassThroughBearerAuthorizer() RequestAuthorizer {
	return RequestAuthorizerFunc(func(request *http.Request, _ Endpoint, authContext RequestAuthContext) error {
		token := strings.TrimSpace(authContext.BearerToken)
		if token == "" {
			return ErrBearerTokenRequired
		}
		request.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
}

func PassThroughBasicAuthorizer() RequestAuthorizer {
	return RequestAuthorizerFunc(func(request *http.Request, _ Endpoint, authContext RequestAuthContext) error {
		username := strings.TrimSpace(authContext.BasicUsername)
		password := strings.TrimSpace(authContext.BasicPassword)
		if username == "" || password == "" {
			return ErrBasicAuthRequired
		}
		rawCredentials := username + ":" + password
		request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(rawCredentials)))
		return nil
	})
}

func StaticBearerAuthorizer(token string) RequestAuthorizer {
	return RequestAuthorizerFunc(func(request *http.Request, endpoint Endpoint, _ RequestAuthContext) error {
		token = strings.TrimSpace(token)
		if token == "" {
			return fmt.Errorf("endpoint %q: %w", endpoint.ID, ErrBearerTokenRequired)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
}

func StaticBasicAuthorizer(username string, password string) RequestAuthorizer {
	return RequestAuthorizerFunc(func(request *http.Request, endpoint Endpoint, _ RequestAuthContext) error {
		username = strings.TrimSpace(username)
		password = strings.TrimSpace(password)
		if username == "" || password == "" {
			return fmt.Errorf("endpoint %q: %w", endpoint.ID, ErrBasicAuthRequired)
		}
		rawCredentials := username + ":" + password
		request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(rawCredentials)))
		return nil
	})
}

func ByEndpointAuthTypeAuthorizer() RequestAuthorizer {
	return RequestAuthorizerFunc(func(request *http.Request, endpoint Endpoint, authContext RequestAuthContext) error {
		switch endpoint.AuthType {
		case AuthTypeNone:
			return nil
		case AuthTypeBearerPassThrough:
			return PassThroughBearerAuthorizer().Authorize(request, endpoint, authContext)
		case AuthTypeBasicPassThrough:
			return PassThroughBasicAuthorizer().Authorize(request, endpoint, authContext)
		case AuthTypeServiceAccount, AuthTypeStaticBearer, AuthTypeStaticBasic:
			return fmt.Errorf("%w: %q", ErrUnsupportedAuthType, endpoint.AuthType)
		default:
			return fmt.Errorf("%w: %q", ErrUnsupportedAuthType, endpoint.AuthType)
		}
	})
}
