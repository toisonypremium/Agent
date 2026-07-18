package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCloudflareAccessVerifierFailsClosed(t *testing.T) {
	v := newCloudflareAccessVerifier()
	v.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	for _, tc := range []struct{ name, token string }{
		{name: "missing", token: ""},
		{name: "malformed", token: "a.b.c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.token != "" {
				req.Header.Set("Cf-Access-Jwt-Assertion", tc.token)
			}
			if _, err := v.verifyRequest(req); err == nil {
				t.Fatal("invalid JWT accepted")
			}
		})
	}
}

func TestCloudflareAccessVerifierRequiresPolicy(t *testing.T) {
	t.Setenv("CF_ACCESS_ISSUER", "")
	t.Setenv("CF_ACCESS_AUDIENCE", "")
	v := newCloudflareAccessVerifier()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cf-Access-Jwt-Assertion", "eyJhbGciOiJSUzI1NiIsImtpZCI6ImsxIn0.eyJpc3MiOiJ4IiwiZXhwIjoxOTAwMDAwMDAwfQ.sig")
	if _, err := v.verifyRequest(req); err == nil {
		t.Fatal("unconfigured JWT policy accepted")
	}
}

func TestCloudflareAccessVerifierAcceptsValidRS256Token(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encodeInt := func(v *big.Int) string { return base64.RawURLEncoding.EncodeToString(v.Bytes()) }
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(accessJWKS{Keys: []accessJWK{{Kty: "RSA", Kid: "kid-1", Alg: "RS256", N: encodeInt(key.N), E: encodeInt(big.NewInt(int64(key.E)))}}})
	}))
	defer jwks.Close()
	t.Setenv("CF_ACCESS_ISSUER", "https://linhbot.cloudflareaccess.com")
	t.Setenv("CF_ACCESS_AUDIENCE", "aud-1")
	t.Setenv("CF_ACCESS_JWKS_URL", jwks.URL)
	now := time.Unix(1700000000, 0).UTC()
	v := newCloudflareAccessVerifier()
	v.now = func() time.Time { return now }
	part := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	head := part(accessJWTHeader{Alg: "RS256", Kid: "kid-1"})
	body := part(accessJWTClaims{Iss: "https://linhbot.cloudflareaccess.com", Aud: []string{"aud-1"}, Email: webOperatorEmail, Exp: now.Add(time.Minute).Unix()})
	digest := sha256.Sum256([]byte(head + "." + body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cf-Access-Jwt-Assertion", head+"."+body+"."+base64.RawURLEncoding.EncodeToString(sig))
	claims, err := v.verifyRequest(req)
	if err != nil || claims.Email != webOperatorEmail {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}
