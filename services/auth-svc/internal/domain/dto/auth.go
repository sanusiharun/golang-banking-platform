// Package dto contains HTTP request and response DTOs for auth-svc.
package dto

// LoginRequest is the payload for POST /auth/login.
type LoginRequest struct {
	Username string `json:"username" validate:"required,min=1,max=100"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

// LoginResponse is returned on successful login or token refresh.
// Both access and refresh tokens are issued together.
type LoginResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	AccessTokenExpiresAt  string `json:"access_token_expires_at"`
	RefreshTokenExpiresAt string `json:"refresh_token_expires_at"`
}

// RefreshRequest is the payload for POST /auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// LogoutRequest is the payload for POST /auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
