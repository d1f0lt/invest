package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/gateway/internal/upstream"
	userspb "invest/backend/services/gateway/internal/userspb"
)

type profileClient struct {
	userspb.UsersServiceClient
	update   *userspb.UpdateMeRequest
	password *userspb.ChangePasswordRequest
	deleted  *userspb.DeleteMeRequest
	err      error
}

func (c *profileClient) UpdateMe(_ context.Context, in *userspb.UpdateMeRequest, _ ...grpc.CallOption) (*userspb.User, error) {
	c.update = in
	if c.err != nil {
		return nil, c.err
	}
	return &userspb.User{Id: "u1", Email: in.GetEmail(), Username: in.Username, CreatedAt: timestamppb.Now()}, nil
}

func (c *profileClient) ChangePassword(_ context.Context, in *userspb.ChangePasswordRequest, _ ...grpc.CallOption) (*userspb.LoginResponse, error) {
	c.password = in
	if c.err != nil {
		return nil, c.err
	}
	return &userspb.LoginResponse{
		AccessToken:           "access",
		ExpiresAt:             timestamppb.Now(),
		UserId:                "u1",
		RefreshToken:          "refresh",
		RefreshTokenExpiresAt: timestamppb.Now(),
	}, nil
}

func (c *profileClient) DeleteMe(_ context.Context, in *userspb.DeleteMeRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	c.deleted = in
	if c.err != nil {
		return nil, c.err
	}
	return &emptypb.Empty{}, nil
}

func doProfile(t *testing.T, client *profileClient, method, path, body string, handler func(*Handlers) http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	h := &Handlers{
		Upstream:        &upstream.Clients{Users: client},
		UpstreamTimeout: time.Second,
		Log:             testLogger(),
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "u1"))
	rec := httptest.NewRecorder()
	handler(h)(rec, req)
	return rec
}

func updateMe(h *Handlers) http.HandlerFunc       { return h.handleUpdateMe }
func changePassword(h *Handlers) http.HandlerFunc { return h.handleChangePassword }
func deleteMe(h *Handlers) http.HandlerFunc       { return h.handleDeleteMe }

func TestHandleUpdateMe_PassesOnlyProvidedFields(t *testing.T) {
	client := &profileClient{}
	rec := doProfile(t, client, http.MethodPatch, "/api/v1/me", `{"username":"andrey"}`, updateMe)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if client.update.Email != nil {
		t.Errorf("email = %v, want nil", client.update.Email)
	}
	if client.update.GetUsername() != "andrey" {
		t.Errorf("username = %q, want andrey", client.update.GetUsername())
	}
	var resp userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Username == nil || *resp.Username != "andrey" {
		t.Errorf("response = %s (err %v)", rec.Body, err)
	}
}

func TestHandleUpdateMe_ConflictIs409(t *testing.T) {
	client := &profileClient{err: status.Error(codes.AlreadyExists, "username already taken")}
	if rec := doProfile(t, client, http.MethodPatch, "/api/v1/me", `{"username":"taken"}`, updateMe); rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestHandleChangePassword_ReturnsNewTokens(t *testing.T) {
	client := &profileClient{}
	rec := doProfile(t, client, http.MethodPost, "/api/v1/me/password", `{"current_password":"old","new_password":"newpassword"}`, changePassword)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if client.password.GetCurrentPassword() != "old" || client.password.GetNewPassword() != "newpassword" {
		t.Errorf("upstream got %+v", client.password)
	}
	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.RefreshToken != "refresh" {
		t.Errorf("response = %s (err %v)", rec.Body, err)
	}
}

func TestHandleChangePassword_WrongPasswordIs403(t *testing.T) {
	client := &profileClient{err: status.Error(codes.PermissionDenied, "wrong password")}
	if rec := doProfile(t, client, http.MethodPost, "/api/v1/me/password", `{"current_password":"x","new_password":"newpassword"}`, changePassword); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleDeleteMe_Returns204(t *testing.T) {
	client := &profileClient{}
	rec := doProfile(t, client, http.MethodDelete, "/api/v1/me", `{"password":"secret"}`, deleteMe)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if client.deleted.GetPassword() != "secret" {
		t.Errorf("upstream got %+v", client.deleted)
	}
}

func TestHandleDeleteMe_WrongPasswordIs403(t *testing.T) {
	client := &profileClient{err: status.Error(codes.PermissionDenied, "wrong password")}
	if rec := doProfile(t, client, http.MethodDelete, "/api/v1/me", `{"password":"x"}`, deleteMe); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
