// Package dto contains HTTP request and response DTOs for auth-svc.
package dto

// LoginRequest is the payload for POST /auth/login.
type LoginRequest struct {
	Username string `json:"username" validate:"required,min=1,max=100"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

// LoginResponse is returned on successful authentication.
// Token is a signed RS256 JWT.  ExpiresAt is RFC3339.
type LoginResponse struct {
	Token     string   `json:"token"`
	ExpiresAt string   `json:"expires_at"`
	UserID    string   `json:"user_id"`
	Roles     []string `json:"roles"`
}
