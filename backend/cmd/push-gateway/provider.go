package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"gateway"
	"gateway/adapter"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultFCMScopeURL = "https://www.googleapis.com/auth/firebase.messaging"
	tokenRefreshSkew   = 2 * time.Minute
)

func providerFromConfig(cfg fileConfig, providerConnectTimeout time.Duration) (gateway.PushProviderCall, string, error) {
	switch cfg.PushProvider {
	case "fcm":
		credentialPath := strings.TrimSpace(os.Getenv("PUSH_FCM_CREDENTIAL_JSON_PATH"))
		if credentialPath != "" {
			scope := strings.TrimSpace(os.Getenv("PUSH_FCM_SCOPE_URL"))
			tokenSource, err := newFCMTokenSource(credentialPath, scope, providerConnectTimeout)
			if err != nil {
				return nil, "", err
			}
			return adapter.PushFCMProviderCallWithTokenSource(
				cfg.PushProviderURL,
				tokenSource.Token,
				providerConnectTimeout,
			), adapter.PushFCMProviderName, nil
		}
		bearerToken := strings.TrimSpace(os.Getenv("PUSH_FCM_BEARER_TOKEN"))
		if bearerToken == "" {
			return nil, "", errors.New("PUSH_FCM_CREDENTIAL_JSON_PATH or PUSH_FCM_BEARER_TOKEN is required for fcm")
		}
		return adapter.PushFCMProviderCall(
			cfg.PushProviderURL,
			bearerToken,
			providerConnectTimeout,
		), adapter.PushFCMProviderName, nil
	default:
		return nil, "", errors.New("pushProvider must be one of: fcm")
	}
}

type serviceAccountJSON struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type serviceAccount struct {
	email      string
	privateKey *rsa.PrivateKey
	tokenURI   string
}

type fcmTokenSource struct {
	mu      sync.Mutex
	token   string
	expiry  time.Time
	account serviceAccount
	scope   string
	client  *http.Client
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type jwtClaims struct {
	Iss   string `json:"iss"`
	Scope string `json:"scope"`
	Aud   string `json:"aud"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
}

func newFCMTokenSource(credentialsPath, scope string, connectTimeout time.Duration) (*fcmTokenSource, error) {
	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, err
	}
	var creds serviceAccountJSON
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, err
	}
	email := strings.TrimSpace(creds.ClientEmail)
	if email == "" {
		return nil, errors.New("client_email is required")
	}
	privateKey := strings.TrimSpace(creds.PrivateKey)
	if privateKey == "" {
		return nil, errors.New("private_key is required")
	}
	tokenURI := strings.TrimSpace(creds.TokenURI)
	if tokenURI == "" {
		return nil, errors.New("token_uri is required")
	}
	key, err := parsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = defaultFCMScopeURL
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	client := &http.Client{Transport: transport}

	return &fcmTokenSource{
		account: serviceAccount{
			email:      email,
			privateKey: key,
			tokenURI:   tokenURI,
		},
		scope:  scope,
		client: client,
	}, nil
}

func (s *fcmTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.token != "" && time.Until(s.expiry) > tokenRefreshSkew {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	token, expiry, err := s.fetchToken(ctx)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.token = token
	s.expiry = expiry
	s.mu.Unlock()

	return token, nil
}

func (s *fcmTokenSource) fetchToken(ctx context.Context) (string, time.Time, error) {
	now := time.Now()
	assertion, err := buildJWT(s.account, s.scope, now)
	if err != nil {
		return "", time.Time{}, err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.account.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", time.Time{}, fmt.Errorf("token request status=%d", resp.StatusCode)
	}

	var token tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", time.Time{}, err
	}
	if token.AccessToken == "" {
		return "", time.Time{}, errors.New("token response missing access_token")
	}
	if token.ExpiresIn <= 0 {
		return "", time.Time{}, errors.New("token response missing expires_in")
	}
	expiry := now.Add(time.Duration(token.ExpiresIn) * time.Second)
	return token.AccessToken, expiry, nil
}

func buildJWT(account serviceAccount, scope string, now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	claims := jwtClaims{
		Iss:   account.email,
		Scope: scope,
		Aud:   account.tokenURI,
		Exp:   now.Add(time.Hour).Unix(),
		Iat:   now.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encoder := base64.RawURLEncoding
	headerPart := encoder.EncodeToString(header)
	payloadPart := encoder.EncodeToString(payload)
	signingInput := headerPart + "." + payloadPart

	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, account.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	signaturePart := encoder.EncodeToString(signature)
	return signingInput + "." + signaturePart, nil
}

func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("private key PEM not found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("private key parse failed")
}
