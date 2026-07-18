package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type accessJWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}
type accessJWTClaims struct {
	Iss   string   `json:"iss"`
	Aud   []string `json:"aud"`
	Email string   `json:"email"`
	Sub   string   `json:"sub"`
	Exp   int64    `json:"exp"`
	Nbf   int64    `json:"nbf"`
}
type accessJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}
type accessJWKS struct {
	Keys []accessJWK `json:"keys"`
}

type cloudflareAccessVerifier struct {
	mu        sync.Mutex
	keys      accessJWKS
	fetchedAt time.Time
	client    *http.Client
	now       func() time.Time
}

func newCloudflareAccessVerifier() *cloudflareAccessVerifier {
	return &cloudflareAccessVerifier{client: &http.Client{Timeout: 5 * time.Second}, now: func() time.Time { return time.Now().UTC() }}
}

func (v *cloudflareAccessVerifier) verifyRequest(r *http.Request) (accessJWTClaims, error) {
	var claims accessJWTClaims
	token := strings.TrimSpace(r.Header.Get("Cf-Access-Jwt-Assertion"))
	if token == "" {
		return claims, fmt.Errorf("Cloudflare Access JWT missing")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, fmt.Errorf("Cloudflare Access JWT malformed")
	}
	var header accessJWTHeader
	if err := decodeJWTPart(parts[0], &header); err != nil || header.Alg != "RS256" || header.Kid == "" {
		return claims, fmt.Errorf("Cloudflare Access JWT header invalid")
	}
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return claims, fmt.Errorf("Cloudflare Access JWT claims invalid")
	}
	now := v.now().Unix()
	if claims.Exp <= now || (claims.Nbf > 0 && claims.Nbf > now+30) {
		return claims, fmt.Errorf("Cloudflare Access JWT expired or not active")
	}
	issuer := strings.TrimRight(strings.TrimSpace(os.Getenv("CF_ACCESS_ISSUER")), "/")
	audience := strings.TrimSpace(os.Getenv("CF_ACCESS_AUDIENCE"))
	if issuer == "" || audience == "" {
		return claims, fmt.Errorf("Cloudflare Access JWT policy not configured")
	}
	if strings.TrimRight(claims.Iss, "/") != issuer || !contains(claims.Aud, audience) {
		return claims, fmt.Errorf("Cloudflare Access JWT issuer/audience mismatch")
	}
	key, err := v.key(header.Kid)
	if err != nil {
		return claims, err
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, fmt.Errorf("Cloudflare Access JWT signature malformed")
	}
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], sig); err != nil {
		return claims, fmt.Errorf("Cloudflare Access JWT signature invalid")
	}
	return claims, nil
}

func (v *cloudflareAccessVerifier) key(kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.fetchedAt.IsZero() || v.now().Sub(v.fetchedAt) > time.Hour {
		url := strings.TrimSpace(os.Getenv("CF_ACCESS_JWKS_URL"))
		if url == "" {
			return nil, fmt.Errorf("Cloudflare Access JWKS URL not configured")
		}
		resp, err := v.client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("fetch Cloudflare Access JWKS: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Cloudflare Access JWKS HTTP %d", resp.StatusCode)
		}
		var keys accessJWKS
		if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
			return nil, fmt.Errorf("decode Cloudflare Access JWKS: %w", err)
		}
		v.keys, v.fetchedAt = keys, v.now()
	}
	for _, jwk := range v.keys.Keys {
		if jwk.Kid == kid && jwk.Kty == "RSA" && (jwk.Alg == "" || jwk.Alg == "RS256") {
			return rsaKey(jwk)
		}
	}
	return nil, fmt.Errorf("Cloudflare Access JWKS key not found")
}

func rsaKey(j accessJWK) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("invalid JWKS modulus encoding")
	}
	eb, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, fmt.Errorf("invalid JWKS exponent encoding")
	}
	n := new(big.Int).SetBytes(nb)
	e := new(big.Int).SetBytes(eb)
	if n.Sign() <= 0 || e.Sign() <= 0 || !e.IsInt64() {
		return nil, fmt.Errorf("invalid JWKS RSA parameters")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}
func decodeJWTPart(part string, out any) error {
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
