package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func Test(t *testing.T) {
	goodPassword := "password"
	goodHash, err := HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("failed to has password: %s", err)
	}

	tests := []struct {
		name      string
		password  string
		hash      string
		wantMatch bool
		wantErr   bool
	}{
		{
			name:      "Correct Password and Hash",
			password:  goodPassword,
			hash:      goodHash,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "Wrong password does not match",
			password:  "WrongPass!",
			hash:      goodHash,
			wantMatch: false,
			wantErr:   false,
		},
		{
			name:      "Invalid Hash",
			password:  goodPassword,
			hash:      "BadHashbrowns",
			wantMatch: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := CheckPasswordHash(tt.password, tt.hash)
			// check err and match against expectations
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if match != tt.wantMatch {
				t.Errorf("match = %v, want %v", err, tt.wantMatch)
			}
		})
	}
}

func TestJWT(t *testing.T) {
	goodUserId := uuid.New()
	goodSecret := "Secret"
	duration, err := time.ParseDuration("30m")
	if err != nil {
		t.Fatalf("Error parsing duration string: %v", err)
	}

	goodJwt, err := MakeJWT(goodUserId, goodSecret, duration)
	if err != nil {
		t.Fatalf("Error creating JTW: %v", err)
	}

	wrongSecretJWT, err := MakeJWT(goodUserId, "Bad-Secret", duration)
	if err != nil {
		t.Fatalf("Error creating badJTW: %v", err)
	}

	zeroDuration, err := time.ParseDuration("-30m")
	if err != nil {
		t.Fatalf("Error creating zero Duration: %v", err)
	}

	expiredJWT, err := MakeJWT(goodUserId, goodSecret, zeroDuration)
	if err != nil {
		t.Fatalf("Error creating expired JWT: %v", err)
	}

	tests := []struct {
		name      string
		jwt       string
		wantValid uuid.UUID
		wantErr   bool
	}{
		{
			name:      "Should be good",
			jwt:       goodJwt,
			wantValid: goodUserId,
			wantErr:   false,
		},
		{
			name:      "Wrong secret",
			jwt:       wrongSecretJWT,
			wantValid: uuid.UUID{},
			wantErr:   true,
		},
		{
			name:      "Expired JTW",
			jwt:       expiredJWT,
			wantValid: uuid.UUID{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := ValidateJWT(tt.jwt, goodSecret)
			// check err and match against expectations
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if valid != tt.wantValid {
				t.Errorf("match = %v, want %v", err, tt.wantValid)
			}
		})
	}
}
