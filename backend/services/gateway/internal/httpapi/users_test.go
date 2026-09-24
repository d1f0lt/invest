package httpapi

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	userspb "invest/backend/services/gateway/internal/userspb"
)

func TestToUserResponse_OmitsAbsentOptionalFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	u := &userspb.User{
		Id:        "u1",
		Email:     "a@example.com",
		CreatedAt: timestamppb.New(now),
	}

	out := toUserResponse(u)
	if out.Username != nil {
		t.Errorf("Username = %v, want nil", out.Username)
	}
	if out.LastLogin != nil {
		t.Errorf("LastLogin = %v, want nil", out.LastLogin)
	}
	if !out.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", out.CreatedAt, now)
	}
}

func TestToUserResponse_CarriesOptionalFieldsWhenPresent(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	username := "andrey"
	u := &userspb.User{
		Id:          "u1",
		Email:       "a@example.com",
		Username:    &username,
		CreatedAt:   timestamppb.New(now),
		LastLoginAt: timestamppb.New(now),
	}

	out := toUserResponse(u)
	if out.Username == nil || *out.Username != "andrey" {
		t.Errorf("Username = %v, want \"andrey\"", out.Username)
	}
	if out.LastLogin == nil || !out.LastLogin.Equal(now) {
		t.Errorf("LastLogin = %v, want %v", out.LastLogin, now)
	}
}

func TestToLoginResponse_CarriesBothTokens(t *testing.T) {
	accessExpires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	refreshExpires := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	r := &userspb.LoginResponse{
		AccessToken:           "access-123",
		ExpiresAt:             timestamppb.New(accessExpires),
		UserId:                "u1",
		RefreshToken:          "refresh-456",
		RefreshTokenExpiresAt: timestamppb.New(refreshExpires),
	}

	out := toLoginResponse(r)
	if out.AccessToken != "access-123" || out.UserID != "u1" {
		t.Errorf("unexpected response: %+v", out)
	}
	if out.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken = %q, want %q", out.RefreshToken, "refresh-456")
	}
	if !out.ExpiresAt.Equal(accessExpires) {
		t.Errorf("ExpiresAt = %v, want %v", out.ExpiresAt, accessExpires)
	}
	if !out.RefreshTokenExpiresAt.Equal(refreshExpires) {
		t.Errorf("RefreshTokenExpiresAt = %v, want %v", out.RefreshTokenExpiresAt, refreshExpires)
	}
}
