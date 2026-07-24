package webconsole

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AccessConfig struct {
	TeamDomain, Audience string
	Client               *http.Client
}
type accessVerifier struct {
	cfg    AccessConfig
	mu     sync.Mutex
	keys   map[string]*rsa.PublicKey
	expiry time.Time
}

func newAccessVerifier(c AccessConfig) (*accessVerifier, error) {
	if c.TeamDomain == "" || c.Audience == "" {
		return nil, fmt.Errorf("Cloudflare Access team domain and audience required")
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 5 * time.Second}
	}
	return &accessVerifier{cfg: c}, nil
}

type accessClaims struct {
	Email    string `json:"email"`
	Subject  string `json:"sub"`
	Audience any    `json:"aud"`
	Exp      int64  `json:"exp"`
	Nbf      int64  `json:"nbf"`
}

func (v *accessVerifier) identity(r *http.Request) (string, error) {
	token := r.Header.Get("Cf-Access-Jwt-Assertion")
	if token == "" {
		return "", fmt.Errorf("Access JWT required")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed Access JWT")
	}
	var head struct{ Kid, Alg string }
	var c accessClaims
	if decode(parts[0], &head) != nil || decode(parts[1], &c) != nil || head.Alg != "RS256" {
		return "", fmt.Errorf("invalid Access JWT")
	}
	key, err := v.key(head.Kid)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], sig) != nil {
		return "", fmt.Errorf("invalid Access JWT signature")
	}
	now := time.Now().Unix()
	if c.Exp <= now || c.Nbf > now || !audienceHas(c.Audience, v.cfg.Audience) {
		return "", fmt.Errorf("Access JWT claims invalid")
	}
	id := strings.TrimSpace(strings.ToLower(c.Email))
	if id == "" {
		id = strings.TrimSpace(c.Subject)
	}
	if id == "" {
		return "", fmt.Errorf("Access identity required")
	}
	return id, nil
}
func decode(raw string, out any) error {
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func exponent(raw []byte) int {
	n := 0
	for _, b := range raw {
		n = n<<8 | int(b)
	}
	return n
}

func audienceHas(a any, want string) bool {
	switch x := a.(type) {
	case string:
		return x == want
	case []any:
		for _, v := range x {
			if s, ok := v.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}
func (v *accessVerifier) key(kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().After(v.expiry) || v.keys == nil {
		resp, e := v.cfg.Client.Get("https://" + v.cfg.TeamDomain + "/cdn-cgi/access/certs")
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		var doc struct{ Keys []struct{ Kid, N, E string } }
		if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&doc) != nil {
			return nil, fmt.Errorf("Access JWKS unavailable")
		}
		v.keys = map[string]*rsa.PublicKey{}
		for _, j := range doc.Keys {
			nb, e := base64.RawURLEncoding.DecodeString(j.N)
			if e != nil {
				continue
			}
			eb, e := base64.RawURLEncoding.DecodeString(j.E)
			if e != nil {
				continue
			}
			v.keys[j.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: exponent(eb)}
		}
		v.expiry = time.Now().Add(time.Hour)
	}
	k := v.keys[kid]
	if k == nil {
		return nil, fmt.Errorf("Access JWT key unknown")
	}
	return k, nil
}
