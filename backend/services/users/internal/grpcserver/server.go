package grpcserver

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"invest/backend/services/users/internal/auth"
	"invest/backend/services/users/internal/storage"
	userspb "invest/backend/services/users/proto"
)

type Store interface {
	CreateUser(ctx context.Context, email string, username *string, passwordHash string) (storage.User, error)
	GetUserByID(ctx context.Context, id string) (storage.User, error)
	GetUserByEmailWithPassword(ctx context.Context, email string) (storage.User, error)
	TouchLastLogin(ctx context.Context, id string) error
	CreateRefreshToken(ctx context.Context, userID, familyID, tokenHash string, expiresAt time.Time) (storage.RefreshToken, error)
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (storage.RefreshToken, error)
	ClaimRefreshToken(ctx context.Context, id string) (bool, error)
	RevokeRefreshTokenFamily(ctx context.Context, familyID string) error
	RevokeRefreshTokenByHash(ctx context.Context, tokenHash string) error
	GetUserByIDWithPassword(ctx context.Context, id string) (storage.User, error)
	UpdateProfile(ctx context.Context, id, email string, username *string) (storage.User, error)
	UpdatePasswordHash(ctx context.Context, id, passwordHash string) error
	RevokeUserRefreshTokens(ctx context.Context, userID string) error
	DeleteUser(ctx context.Context, id string) error
}

type Server struct {
	userspb.UnimplementedUsersServiceServer

	Store           Store
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	BcryptCost      int
	Log             *slog.Logger
}

var emailRE = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

const minPasswordLength = 8

const dummyBcryptHash = "$2a$12$CwTycUXWue0Thq9StjUM0uJ8Q1c/1mCEB4nJfP.KRJ8bK8bRN9k9O"

const invalidRefreshToken = "invalid or expired refresh token"

const reuseGracePeriod = 30 * time.Second

func (s *Server) Register(ctx context.Context, req *userspb.RegisterRequest) (*userspb.User, error) {
	email := strings.ToLower(strings.TrimSpace(req.GetEmail()))
	if !emailRE.MatchString(email) {
		return nil, status.Error(codes.InvalidArgument, "invalid email")
	}
	if len(req.GetPassword()) < minPasswordLength {
		return nil, status.Error(codes.InvalidArgument, "password must be at least 8 characters")
	}

	var username *string
	if req.Username != nil {
		trimmed := strings.TrimSpace(req.GetUsername())
		if trimmed != "" {
			username = &trimmed
		}
	}

	hash, err := auth.HashPassword(req.GetPassword(), s.BcryptCost)
	if err != nil {
		s.Log.Error("hash password", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	user, err := s.Store.CreateUser(ctx, email, username, hash)
	switch {
	case errors.Is(err, storage.ErrEmailTaken):
		return nil, status.Error(codes.AlreadyExists, "email already registered")
	case errors.Is(err, storage.ErrUsernameTaken):
		return nil, status.Error(codes.AlreadyExists, "username already taken")
	case err != nil:
		s.Log.Error("create user", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	return toUserPB(user), nil
}

func (s *Server) Login(ctx context.Context, req *userspb.LoginRequest) (*userspb.LoginResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.GetEmail()))

	const invalidCreds = "invalid email or password"

	user, err := s.Store.GetUserByEmailWithPassword(ctx, email)
	if errors.Is(err, storage.ErrNotFound) {

		auth.VerifyPassword(dummyBcryptHash, req.GetPassword())
		return nil, status.Error(codes.Unauthenticated, invalidCreds)
	}
	if err != nil {
		s.Log.Error("get user by email", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if !user.IsActive || !auth.VerifyPassword(user.PasswordHash, req.GetPassword()) {
		return nil, status.Error(codes.Unauthenticated, invalidCreds)
	}

	resp, err := s.issueTokenPair(ctx, user.ID, "")
	if err != nil {
		return nil, err
	}

	if err := s.Store.TouchLastLogin(ctx, user.ID); err != nil {
		s.Log.Warn("touch last_login_at", "error", err, "user_id", user.ID)
	}

	return resp, nil
}

func (s *Server) RefreshToken(ctx context.Context, req *userspb.RefreshTokenRequest) (*userspb.LoginResponse, error) {
	rawToken := req.GetRefreshToken()
	if rawToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	tokenHash := auth.HashRefreshToken(rawToken)

	stored, err := s.Store.GetRefreshTokenByHash(ctx, tokenHash)
	if errors.Is(err, storage.ErrRefreshTokenNotFound) {
		return nil, status.Error(codes.Unauthenticated, invalidRefreshToken)
	}
	if err != nil {
		s.Log.Error("get refresh token by hash", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	if stored.RevokedAt != nil {
		if time.Since(*stored.RevokedAt) > reuseGracePeriod {
			s.Log.Warn("refresh token reuse detected; revoking family", "user_id", stored.UserID, "family_id", stored.FamilyID, "revoked_at", stored.RevokedAt)
			if err := s.Store.RevokeRefreshTokenFamily(ctx, stored.FamilyID); err != nil {
				s.Log.Error("revoke refresh token family on reuse", "error", err)
				return nil, status.Error(codes.Internal, "internal error")
			}
		}
		return nil, status.Error(codes.Unauthenticated, invalidRefreshToken)
	}

	if !time.Now().Before(stored.ExpiresAt) {
		return nil, status.Error(codes.Unauthenticated, invalidRefreshToken)
	}

	user, err := s.Store.GetUserByID(ctx, stored.UserID)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && !user.IsActive) {
		return nil, status.Error(codes.Unauthenticated, invalidRefreshToken)
	}
	if err != nil {
		s.Log.Error("get user by id", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	claimed, err := s.Store.ClaimRefreshToken(ctx, stored.ID)
	if err != nil {
		s.Log.Error("claim refresh token", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if !claimed {
		return nil, status.Error(codes.Unauthenticated, invalidRefreshToken)
	}

	return s.issueTokenPair(ctx, user.ID, stored.FamilyID)
}

func (s *Server) Logout(ctx context.Context, req *userspb.LogoutRequest) (*emptypb.Empty, error) {
	rawToken := req.GetRefreshToken()
	if rawToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	tokenHash := auth.HashRefreshToken(rawToken)
	if err := s.Store.RevokeRefreshTokenByHash(ctx, tokenHash); err != nil {
		s.Log.Error("revoke refresh token by hash", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &emptypb.Empty{}, nil
}

func (s *Server) issueTokenPair(ctx context.Context, userID, familyID string) (*userspb.LoginResponse, error) {
	accessToken, accessExpiresAt, err := auth.IssueToken(s.JWTSecret, userID, s.AccessTokenTTL)
	if err != nil {
		s.Log.Error("issue access token", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		s.Log.Error("generate refresh token", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	refreshExpiresAt := time.Now().Add(s.RefreshTokenTTL)

	if _, err := s.Store.CreateRefreshToken(ctx, userID, familyID, auth.HashRefreshToken(refreshToken), refreshExpiresAt); err != nil {
		s.Log.Error("store refresh token", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &userspb.LoginResponse{
		AccessToken:           accessToken,
		ExpiresAt:             timestamppb.New(accessExpiresAt),
		UserId:                userID,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: timestamppb.New(refreshExpiresAt),
	}, nil
}

func (s *Server) GetMe(ctx context.Context, _ *emptypb.Empty) (*userspb.User, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}
	return s.respondWithUser(ctx, userID)
}

func (s *Server) GetUser(ctx context.Context, req *userspb.GetUserRequest) (*userspb.User, error) {

	if _, err := auth.UserID(ctx); err != nil {
		return nil, err
	}
	return s.respondWithUser(ctx, req.GetId())
}

func (s *Server) UpdateMe(ctx context.Context, req *userspb.UpdateMeRequest) (*userspb.User, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return nil, err
	}

	current, err := s.Store.GetUserByID(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if err != nil {
		s.Log.Error("get user by id", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	email := current.Email
	if req.Email != nil {
		email = strings.ToLower(strings.TrimSpace(req.GetEmail()))
		if !emailRE.MatchString(email) {
			return nil, status.Error(codes.InvalidArgument, "invalid email")
		}
	}

	username := current.Username
	if req.Username != nil {
		username = nil
		if trimmed := strings.TrimSpace(req.GetUsername()); trimmed != "" {
			username = &trimmed
		}
	}

	user, err := s.Store.UpdateProfile(ctx, userID, email, username)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, status.Error(codes.NotFound, "user not found")
	case errors.Is(err, storage.ErrEmailTaken):
		return nil, status.Error(codes.AlreadyExists, "email already registered")
	case errors.Is(err, storage.ErrUsernameTaken):
		return nil, status.Error(codes.AlreadyExists, "username already taken")
	case err != nil:
		s.Log.Error("update profile", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return toUserPB(user), nil
}

func (s *Server) ChangePassword(ctx context.Context, req *userspb.ChangePasswordRequest) (*userspb.LoginResponse, error) {
	user, err := s.verifiedCaller(ctx, req.GetCurrentPassword())
	if err != nil {
		return nil, err
	}
	if len(req.GetNewPassword()) < minPasswordLength {
		return nil, status.Error(codes.InvalidArgument, "password must be at least 8 characters")
	}

	hash, err := auth.HashPassword(req.GetNewPassword(), s.BcryptCost)
	if err != nil {
		s.Log.Error("hash password", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if err := s.Store.UpdatePasswordHash(ctx, user.ID, hash); err != nil {
		s.Log.Error("update password hash", "error", err, "user_id", user.ID)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if err := s.Store.RevokeUserRefreshTokens(ctx, user.ID); err != nil {
		s.Log.Error("revoke user refresh tokens", "error", err, "user_id", user.ID)
		return nil, status.Error(codes.Internal, "internal error")
	}

	return s.issueTokenPair(ctx, user.ID, "")
}

func (s *Server) DeleteMe(ctx context.Context, req *userspb.DeleteMeRequest) (*emptypb.Empty, error) {
	user, err := s.verifiedCaller(ctx, req.GetPassword())
	if err != nil {
		return nil, err
	}

	err = s.Store.DeleteUser(ctx, user.ID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.Log.Error("delete user", "error", err, "user_id", user.ID)
		return nil, status.Error(codes.Internal, "internal error")
	}
	s.Log.Info("user deleted", "user_id", user.ID)
	return &emptypb.Empty{}, nil
}

func (s *Server) verifiedCaller(ctx context.Context, password string) (storage.User, error) {
	userID, err := auth.UserID(ctx)
	if err != nil {
		return storage.User{}, err
	}

	user, err := s.Store.GetUserByIDWithPassword(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.User{}, status.Error(codes.NotFound, "user not found")
	}
	if err != nil {
		s.Log.Error("get user with password by id", "error", err)
		return storage.User{}, status.Error(codes.Internal, "internal error")
	}
	if !auth.VerifyPassword(user.PasswordHash, password) {
		return storage.User{}, status.Error(codes.PermissionDenied, "wrong password")
	}
	return user, nil
}

func (s *Server) respondWithUser(ctx context.Context, id string) (*userspb.User, error) {
	user, err := s.Store.GetUserByID(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if err != nil {
		s.Log.Error("get user by id", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return toUserPB(user), nil
}

func toUserPB(u storage.User) *userspb.User {
	out := &userspb.User{
		Id:        u.ID,
		Email:     u.Email,
		Username:  u.Username,
		CreatedAt: timestamppb.New(u.CreatedAt),
	}
	if u.LastLoginAt != nil {
		out.LastLoginAt = timestamppb.New(*u.LastLoginAt)
	}
	return out
}
