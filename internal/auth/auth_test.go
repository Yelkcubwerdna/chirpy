package auth

import (
	"testing"
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
