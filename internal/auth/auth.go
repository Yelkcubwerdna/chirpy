package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	hashed_password, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", fmt.Errorf("Error hasing password: %s", err)
	}
	return hashed_password, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	valid, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("Error comparing password and hash: %s", err)
	}

	return valid, err
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject:   userID.String(),
	})

	tString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", fmt.Errorf("Error creating token string: %v", err)
	}

	return tString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	type CustomClaims struct {
		Issuer    string          `json:"issuer"`
		IssuedAt  jwt.NumericDate `json:"issued_at"`
		ExpiresAt jwt.NumericDate `json:"expires_at"`
		Subject   string          `json:"subject"`
		jwt.RegisteredClaims
	}

	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("Error parsing token: %v", err)
	}

	tokenClaims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return uuid.UUID{}, fmt.Errorf("Error with type assertion: %v", err)
	}

	userId, err := uuid.Parse(tokenClaims.Subject)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("Error parsing stringified UUID: %v", err)
	}

	return userId, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	// Get bearer token
	bearer := headers.Get("Authorization")

	// strip off Bearer prefix and whitespace
	bearer, ok := strings.CutPrefix(bearer, "Bearer ")
	// If it didn't have the prefic something has gone wrong
	if !ok {
		return "", fmt.Errorf("No authorization value in header")
	}

	return bearer, nil
}

func MakeRefreshToken() string {
	data := make([]byte, 32)
	rand.Read(data)
	token := hex.EncodeToString(data)
	return token
}
