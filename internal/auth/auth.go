package auth

import (
	"fmt"

	"github.com/alexedwards/argon2id"
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
