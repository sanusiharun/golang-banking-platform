package unit

import (
	"context"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
)

// ── Mock UserRepository ────────────────────────────────────────────────────────

type MockUserRepo struct {
	User *dao.User
	Err  error

	FindByUsernameCalls int
	FindByIDCalls       int
}

func (m *MockUserRepo) FindByUsername(_ context.Context, _ string) (*dao.User, error) {
	m.FindByUsernameCalls++
	return m.User, m.Err
}

func (m *MockUserRepo) FindByID(_ context.Context, _ string) (*dao.User, error) {
	m.FindByIDCalls++
	return m.User, m.Err
}

var _ repository.UserRepository = (*MockUserRepo)(nil)

// ── Mock TokenStore ────────────────────────────────────────────────────────────

type MockTokenStore struct {
	SaveErr        error
	FindToken      *dao.RefreshToken
	FindErr        error
	RevokeErr      error
	SaveCalls      int
	FindCalls      int
	RevokeCalls    int
	RevokeAllCalls int
}

func (m *MockTokenStore) Save(_ context.Context, _ *dao.RefreshToken) error {
	m.SaveCalls++
	return m.SaveErr
}

func (m *MockTokenStore) FindByHash(_ context.Context, _ string) (*dao.RefreshToken, error) {
	m.FindCalls++
	return m.FindToken, m.FindErr
}

func (m *MockTokenStore) Revoke(_ context.Context, _ string) error {
	m.RevokeCalls++
	return m.RevokeErr
}

func (m *MockTokenStore) RevokeAllForUser(_ context.Context, _ string) error {
	m.RevokeAllCalls++
	return m.RevokeErr
}

var _ repository.TokenStore = (*MockTokenStore)(nil)
