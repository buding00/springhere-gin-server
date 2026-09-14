package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/fast-template/springhere-gin-server/pkg/config"
	"github.com/golang-jwt/jwt/v5"
)

type Manager struct {
	secret           []byte
	issuer, audience string
	ttl              time.Duration
}

// AccessIdentity is the verified identity encoded in an access token.
type AccessIdentity struct {
	UserID    string
	SessionID string
	AccessJTI string
}

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func NewJWTManager(cfg config.AuthConfig) Manager {
	return Manager{secret: []byte(cfg.JWTSecret), issuer: cfg.Issuer, audience: cfg.Audience, ttl: cfg.AccessTokenTTL}
}

func (m Manager) Issue(userID, sessionID, accessJTI string) (string, error) {
	now := time.Now()
	claims := accessClaims{SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{Issuer: m.issuer, Subject: userID, Audience: jwt.ClaimStrings{m.audience}, ID: accessJTI, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}
func (m Manager) Parse(raw string) (AccessIdentity, error) {
	claims := &accessClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithAudience(m.audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid || claims.Subject == "" || claims.ID == "" || claims.IssuedAt == nil {
		return AccessIdentity{}, fmt.Errorf("invalid access token")
	}
	if claims.SessionID == "" {
		return AccessIdentity{}, fmt.Errorf("missing access token session")
	}
	return AccessIdentity{UserID: claims.Subject, SessionID: claims.SessionID, AccessJTI: claims.ID}, nil
}
func Random() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func HashToken(raw string) []byte { sum := sha256.Sum256([]byte(raw)); return sum[:] }

// HashTokenHex returns the fixed-length digest used only as a Redis key and
// session field. Raw Access and Refresh Tokens are never persisted.
func HashTokenHex(raw string) string { return hex.EncodeToString(HashToken(raw)) }
