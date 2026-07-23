package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJWTAuthenticatorChecksPassword(t *testing.T) {
	authenticator := NewJWTAuthenticator(AuthConfig{
		SecretKey:   "test-secret",
		TokenExpiry: time.Hour,
	})

	user := &User{
		ID:        "user-1",
		Username:  "alice",
		Email:     "alice@example.com",
		Role:      UserRole,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := authenticator.RegisterUser(user, "correct-password"); err != nil {
		t.Fatalf("RegisterUser() error = %v", err)
	}

	if _, err := authenticator.Authenticate(context.Background(), "alice", "correct-password"); err != nil {
		t.Fatalf("Authenticate() with correct password error = %v", err)
	}

	gotUser, err := authenticator.Authenticate(context.Background(), "alice", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Authenticate() with wrong password error = %v, want %v", err, ErrInvalidCredentials)
	}
	if gotUser != nil {
		t.Fatalf("Authenticate() with wrong password user = %v, want nil", gotUser)
	}
}
