package httpapi

import (
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"invest/backend/services/gateway/internal/auth"
	userspb "invest/backend/services/gateway/internal/userspb"
)

type registerRequest struct {
	Email    string  `json:"email"`
	Username *string `json:"username,omitempty"`
	Password string  `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type updateMeRequest struct {
	Email    *string `json:"email,omitempty"`
	Username *string `json:"username,omitempty"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type deleteMeRequest struct {
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken           string    `json:"access_token"`
	ExpiresAt             time.Time `json:"expires_at"`
	UserID                string    `json:"user_id"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

type userResponse struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Username  *string    `json:"username,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	LastLogin *time.Time `json:"last_login_at,omitempty"`
}

func toUserResponse(u *userspb.User) userResponse {
	out := userResponse{
		ID:        u.GetId(),
		Email:     u.GetEmail(),
		Username:  u.Username,
		CreatedAt: u.GetCreatedAt().AsTime(),
	}
	if u.LastLoginAt != nil {
		t := u.GetLastLoginAt().AsTime()
		out.LastLogin = &t
	}
	return out
}

func toLoginResponse(r *userspb.LoginResponse) loginResponse {
	return loginResponse{
		AccessToken:           r.GetAccessToken(),
		ExpiresAt:             r.GetExpiresAt().AsTime(),
		UserID:                r.GetUserId(),
		RefreshToken:          r.GetRefreshToken(),
		RefreshTokenExpiresAt: r.GetRefreshTokenExpiresAt().AsTime(),
	}
}

func (h *Handlers) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()

	user, err := h.Upstream.Users.Register(ctx, &userspb.RegisterRequest{
		Email:    req.Email,
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserResponse(user))
}

func (h *Handlers) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()

	resp, err := h.Upstream.Users.Login(ctx, &userspb.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toLoginResponse(resp))
}

func (h *Handlers) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()

	resp, err := h.Upstream.Users.RefreshToken(ctx, &userspb.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toLoginResponse(resp))
}

func (h *Handlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	ctx, cancel := h.callCtx(r)
	defer cancel()

	if _, err := h.Upstream.Users.Logout(ctx, &userspb.LogoutRequest{
		RefreshToken: req.RefreshToken,
	}); err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	user, err := h.Upstream.Users.GetMe(ctx, &emptypb.Empty{})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handlers) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	user, err := h.Upstream.Users.GetUser(ctx, &userspb.GetUserRequest{Id: r.PathValue("id")})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handlers) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var req updateMeRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	user, err := h.Upstream.Users.UpdateMe(ctx, &userspb.UpdateMeRequest{
		Email:    req.Email,
		Username: req.Username,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handlers) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	resp, err := h.Upstream.Users.ChangePassword(ctx, &userspb.ChangePasswordRequest{
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
	})
	if err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, toLoginResponse(resp))
}

func (h *Handlers) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	var req deleteMeRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	userID, _ := userIDFromContext(r.Context())
	ctx, cancel := h.callCtx(r)
	defer cancel()
	ctx = auth.WithUserID(ctx, userID)

	if _, err := h.Upstream.Users.DeleteMe(ctx, &userspb.DeleteMeRequest{
		Password: req.Password,
	}); err != nil {
		writeUpstreamError(w, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
