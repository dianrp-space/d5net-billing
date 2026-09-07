package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims stores user/tenant IDs as UUID strings for JWT compatibility.
type Claims struct {
	UserID   string `json:"uid"`
	TenantID string `json:"tid"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	Type     string `json:"typ"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenService(secret string, accessTTL, refreshTTL time.Duration) *TokenService {
	return &TokenService{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (s *TokenService) CreateAccessToken(userID, tenantID uuid.UUID, email, role string) (string, time.Time, error) {
	exp := time.Now().Add(s.accessTTL)
	uid := userID.String()
	tid := tenantID.String()
	if tenantID == uuid.Nil {
		tid = uuid.Nil.String()
	}
	claims := Claims{
		UserID:   uid,
		TenantID: tid,
		Role:     role,
		Email:    email,
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   uid,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	return signed, exp, err
}

func (s *TokenService) CreateRefreshToken(userID uuid.UUID) (string, time.Time, error) {
	exp := time.Now().Add(s.refreshTTL)
	uid := userID.String()
	claims := Claims{
		UserID: uid,
		Type:   "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   uid,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	return signed, exp, err
}

func (s *TokenService) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *TokenService) CreatePortalToken(tenantID uuid.UUID, phone string) (string, error) {
	ttl := s.accessTTL
	if ttl < 8*time.Hour {
		ttl = 8 * time.Hour
	}
	exp := time.Now().Add(ttl)
	claims := Claims{
		TenantID: tenantID.String(),
		Email:    phone,
		Role:     "portal",
		Type:     "portal",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   phone,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	return signed, err
}
