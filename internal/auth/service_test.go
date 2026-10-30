package auth

import (
	"context"
	"e-commerce/pkg/errno"
	"encoding/base64"
	"github.com/google/uuid"
	"testing"
)

func TestOpaqueTokens(t *testing.T) {
	svc := &Service{}
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		token, err := svc.generateToken(uuid.Nil, "", AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || len(raw) != 32 || len(token) != 43 {
			t.Fatalf("invalid token format")
		}
		if seen[token] {
			t.Fatal("duplicate token")
		}
		seen[token] = true
	}
}

func TestRejectMalformedTokenBeforeRedis(t *testing.T) {
	svc := &Service{}
	for _, token := range []string{"", "fake.token.value", "Bearer abc", "short", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAB"} {
		if _, err := svc.VerifyToken(context.Background(), token, AccessToken); err != errno.ErrAuthInvalidToken {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if _, err := svc.VerifyToken(context.Background(), "", TokenType("unknown")); err != errno.ErrAuthInvalidToken {
		t.Fatal(err)
	}
}
