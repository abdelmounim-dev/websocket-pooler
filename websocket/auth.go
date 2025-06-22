package websocket

import (
	"context"
	"fmt"

	"github.com/abdelmounim-dev/websocket-pooler/config"
	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v5"
)

// CustomClaims defines the structure of the JWT claims used in the system.
// It includes standard claims and custom ones like `scopes`.
// The 'jti' (JWT ID) from RegisteredClaims is crucial for token revocation.
type CustomClaims struct {
	Scopes []string `json:"scopes"`
	jwt.RegisteredClaims
}

// JWTValidator handles JWT validation logic.
type JWTValidator struct {
	cfg         *config.AuthConfig
	redisClient *redis.Client
}

// NewJWTValidator creates a new JWT validator.
func NewJWTValidator(cfg *config.AuthConfig, redisClient *redis.Client) *JWTValidator {
	return &JWTValidator{
		cfg:         cfg,
		redisClient: redisClient,
	}
}

// ValidateToken parses and validates a JWT string. It checks the signature,
// standard claims (like expiration), and the revocation list in Redis.
func (v *JWTValidator) ValidateToken(ctx context.Context, tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(v.cfg.JWTSecret), nil
	})

	if err != nil {
		// This handles parsing errors, signature validation errors, and expired tokens.
		return nil, fmt.Errorf("token parse/validation error: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok {
		return nil, fmt.Errorf("could not cast claims to CustomClaims")
	}

	// Check if the token has been revoked
	isRevoked, err := v.isTokenRevoked(ctx, claims.ID)
	if err != nil {
		// Log the error but don't block login, to prevent Redis outage from blocking all users.
		log.Printf("CRITICAL: Failed to check token revocation status: %v", err)
	}
	if isRevoked {
		return nil, fmt.Errorf("token has been revoked")
	}

	return claims, nil
}

// isTokenRevoked checks if a token ID (JTI) is in the Redis revocation list.
func (v *JWTValidator) isTokenRevoked(ctx context.Context, jti string) (bool, error) {
	if v.redisClient == nil || jti == "" {
		// If no Redis or JTI, we cannot check revocation.
		// This is a "fail-open" approach. Log if JTI is missing.
		if jti == "" {
			log.Println("Warning: JWT token is missing 'jti' claim, cannot check for revocation.")
		}
		return false, nil
	}

	key := fmt.Sprintf("%s:%s", v.cfg.RevocationListKey, jti)
	exists, err := v.redisClient.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis command failed: %w", err)
	}

	return exists == 1, nil
}
