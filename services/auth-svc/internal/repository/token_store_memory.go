package repository

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

// memoryTokenStore is an in-memory TokenStore for local dev and testing only.
// All data is lost on restart — never use in production.
type memoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*dao.RefreshToken // hash → token
}

func NewMemoryTokenStore() TokenStore {
	return &memoryTokenStore{
		tokens: make(map[string]*dao.RefreshToken),
	}
}

func (s *memoryTokenStore) Save(_ context.Context, rt *dao.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *rt
	s.tokens[rt.TokenHash] = &cp
	return nil
}

func (s *memoryTokenStore) FindByHash(ctx context.Context, hash string) (*dao.RefreshToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rt, ok := s.tokens[hash]
	if !ok {
		return nil, ErrTokenNotFound
	}
	if rt.Revoked {
		slog.WarnContext(ctx, "token_store: revoked token used", slog.String("user_id", rt.UserID))
		return nil, ErrTokenRevoked
	}
	if time.Now().UTC().After(rt.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	cp := *rt
	return &cp, nil
}

func (s *memoryTokenStore) Revoke(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rt, ok := s.tokens[hash]; ok {
		rt.Revoked = true
	}
	return nil
}

func (s *memoryTokenStore) RevokeAllForUser(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, rt := range s.tokens {
		if rt.UserID == userID && !rt.Revoked {
			rt.Revoked = true
			count++
		}
	}
	slog.InfoContext(ctx, "token_store: all tokens revoked for user",
		slog.String("user_id", userID),
		slog.Int("count", count),
	)
	return nil
}
