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
