package grpcserver

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"invest/backend/services/users/internal/auth"
	"invest/backend/services/users/internal/storage"
	userspb "invest/backend/services/users/proto"
)

type fakeStore struct {
	mu            sync.Mutex
	byID          map[string]storage.User
	byEmail       map[string]storage.User
	refreshTokens map[string]storage.RefreshToken
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		byID:          map[string]storage.User{},
		byEmail:       map[string]storage.User{},
		refreshTokens: map[string]storage.RefreshToken{},
	}
}

func (f *fakeStore) CreateUser(_ context.Context, email string, username *string, passwordHash string) (storage.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byEmail[email]; ok {
		return storage.User{}, storage.ErrEmailTaken
	}
	u := storage.User{ID: "id-" + email, Email: email, Username: username, PasswordHash: passwordHash, IsActive: true, CreatedAt: time.Now()}
	f.byID[u.ID] = u
	f.byEmail[email] = u
	return u, nil
}

func (f *fakeStore) GetUserByID(_ context.Context, id string) (storage.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return storage.User{}, storage.ErrNotFound
}

func (f *fakeStore) GetUserByEmailWithPassword(_ context.Context, email string) (storage.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return storage.User{}, storage.ErrNotFound
}

func (f *fakeStore) TouchLastLogin(_ context.Context, _ string) error { return nil }

func (f *fakeStore) CreateRefreshToken(_ context.Context, userID, familyID, tokenHash string, expiresAt time.Time) (storage.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if familyID == "" {
		familyID = "fam-" + tokenHash
	}
	rt := storage.RefreshToken{ID: "rt-" + tokenHash, UserID: userID, FamilyID: familyID, TokenHash: tokenHash, ExpiresAt: expiresAt, CreatedAt: time.Now()}
	f.refreshTokens[tokenHash] = rt
	return rt, nil
}

func (f *fakeStore) GetRefreshTokenByHash(_ context.Context, tokenHash string) (storage.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rt, ok := f.refreshTokens[tokenHash]; ok {
		return rt, nil
	}
	return storage.RefreshToken{}, storage.ErrRefreshTokenNotFound
}

func (f *fakeStore) ClaimRefreshToken(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for hash, rt := range f.refreshTokens {
		if rt.ID == id && rt.RevokedAt == nil && time.Now().Before(rt.ExpiresAt) {
			now := time.Now()
			rt.RevokedAt = &now
			f.refreshTokens[hash] = rt
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) RevokeRefreshTokenFamily(_ context.Context, familyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for hash, rt := range f.refreshTokens {
		if rt.FamilyID == familyID && rt.RevokedAt == nil {
			rt.RevokedAt = &now
			f.refreshTokens[hash] = rt
		}
	}
	return nil
}

func (f *fakeStore) RevokeRefreshTokenByHash(_ context.Context, tokenHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rt, ok := f.refreshTokens[tokenHash]; ok {
		now := time.Now()
		rt.RevokedAt = &now
		f.refreshTokens[tokenHash] = rt
	}
	return nil
}

func (f *fakeStore) GetUserByIDWithPassword(ctx context.Context, id string) (storage.User, error) {
	return f.GetUserByID(ctx, id)
}

func (f *fakeStore) UpdateProfile(_ context.Context, id, email string, username *string) (storage.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return storage.User{}, storage.ErrNotFound
	}
	if other, taken := f.byEmail[email]; taken && other.ID != id {
		return storage.User{}, storage.ErrEmailTaken
	}
	if username != nil {
		for _, other := range f.byID {
			if other.ID != id && other.Username != nil && strings.EqualFold(*other.Username, *username) {
				return storage.User{}, storage.ErrUsernameTaken
			}
		}
	}
	delete(f.byEmail, u.Email)
	u.Email = email
	u.Username = username
	f.byID[id] = u
	f.byEmail[email] = u
	return u, nil
}

func (f *fakeStore) UpdatePasswordHash(_ context.Context, id, passwordHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return storage.ErrNotFound
	}
	u.PasswordHash = passwordHash
	f.byID[id] = u
	f.byEmail[u.Email] = u
	return nil
}

func (f *fakeStore) RevokeUserRefreshTokens(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for hash, rt := range f.refreshTokens {
		if rt.UserID == userID && rt.RevokedAt == nil {
			rt.RevokedAt = &now
			f.refreshTokens[hash] = rt
		}
	}
	return nil
}

func (f *fakeStore) DeleteUser(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return storage.ErrNotFound
	}
	delete(f.byID, id)
	delete(f.byEmail, u.Email)
	for hash, rt := range f.refreshTokens {
		if rt.UserID == id {
			delete(f.refreshTokens, hash)
		}
	}
	return nil
}

func (f *fakeStore) setRevokedAt(rawToken string, revokedAt time.Time) {
	hash := auth.HashRefreshToken(rawToken)
	f.mu.Lock()
	defer f.mu.Unlock()
	rt := f.refreshTokens[hash]
	rt.RevokedAt = &revokedAt
	f.refreshTokens[hash] = rt
}

func newTestServer(store Store) *Server {
	return &Server{
		Store:           store,
		JWTSecret:       []byte("test-secret-at-least-32-bytes-long!"),
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 24 * time.Hour,
		BcryptCost:      4,
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func withUserID(id string) context.Context {
	md := metadata.Pairs(auth.UserIDKey, id)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestRegister_Success(t *testing.T) {
	s := newTestServer(newFakeStore())
	u, err := s.Register(context.Background(), &userspb.RegisterRequest{Email: "A@Example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if u.Email != "a@example.com" {
		t.Errorf("email = %q, want lowercased %q", u.Email, "a@example.com")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	req := &userspb.RegisterRequest{Email: "a@example.com", Password: "correct horse battery staple"}
	if _, err := s.Register(context.Background(), req); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := s.Register(context.Background(), req)
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("code = %v, want AlreadyExists", status.Code(err))
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.Register(context.Background(), &userspb.RegisterRequest{Email: "not-an-email", Password: "correct horse battery staple"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.Register(context.Background(), &userspb.RegisterRequest{Email: "a@example.com", Password: "short"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestLogin_Success(t *testing.T) {
	store := newFakeStore()
	hash, err := auth.HashPassword("correct horse battery staple", 4)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	store.byEmail["a@example.com"] = storage.User{ID: "u1", Email: "a@example.com", PasswordHash: hash, IsActive: true}
	store.byID["u1"] = store.byEmail["a@example.com"]

	s := newTestServer(store)
	resp, err := s.Login(context.Background(), &userspb.LoginRequest{Email: "a@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.UserId != "u1" || resp.AccessToken == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.RefreshToken == "" {
		t.Errorf("expected non-empty refresh_token")
	}
	if resp.RefreshTokenExpiresAt == nil {
		t.Errorf("expected refresh_token_expires_at to be set")
	}
	store.mu.Lock()
	n := len(store.refreshTokens)
	store.mu.Unlock()
	if n != 1 {
		t.Errorf("refresh tokens stored = %d, want 1", n)
	}
}

func TestLogin_WrongPasswordAndUnknownEmailGiveSameError(t *testing.T) {
	store := newFakeStore()
	hash, _ := auth.HashPassword("correct horse battery staple", 4)
	store.byEmail["a@example.com"] = storage.User{ID: "u1", Email: "a@example.com", PasswordHash: hash, IsActive: true}
	s := newTestServer(store)

	_, err1 := s.Login(context.Background(), &userspb.LoginRequest{Email: "a@example.com", Password: "wrong"})
	_, err2 := s.Login(context.Background(), &userspb.LoginRequest{Email: "nobody@example.com", Password: "whatever"})

	if status.Code(err1) != codes.Unauthenticated || status.Code(err2) != codes.Unauthenticated {
		t.Fatalf("codes = %v, %v, want both Unauthenticated", status.Code(err1), status.Code(err2))
	}
	if st1, st2 := status.Convert(err1), status.Convert(err2); st1.Message() != st2.Message() {
		t.Errorf("messages differ (user enumeration risk): %q vs %q", st1.Message(), st2.Message())
	}
}

func TestGetMe_RequiresMetadata(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.GetMe(context.Background(), &emptypb.Empty{})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestGetMe_Success(t *testing.T) {
	store := newFakeStore()
	store.byID["u1"] = storage.User{ID: "u1", Email: "a@example.com", CreatedAt: time.Now()}
	s := newTestServer(store)

	u, err := s.GetMe(withUserID("u1"), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if u.Id != "u1" {
		t.Errorf("id = %q, want u1", u.Id)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.GetUser(withUserID("caller"), &userspb.GetUserRequest{Id: "nonexistent"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func loginTestUser(t *testing.T, s *Server, store *fakeStore) *userspb.LoginResponse {
	t.Helper()
	hash, err := auth.HashPassword("correct horse battery staple", 4)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	store.byEmail["a@example.com"] = storage.User{ID: "u1", Email: "a@example.com", PasswordHash: hash, IsActive: true}
	store.byID["u1"] = store.byEmail["a@example.com"]

	resp, err := s.Login(context.Background(), &userspb.LoginRequest{Email: "a@example.com", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return resp
}

func TestRefreshToken_Success(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	resp, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if resp.UserId != "u1" || resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.RefreshToken == login.RefreshToken {
		t.Errorf("expected a rotated (new) refresh token")
	}
}

func TestRefreshToken_RotationRejectsOldToken(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	if _, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken}); err != nil {
		t.Fatalf("first RefreshToken: %v", err)
	}

	_, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated (reused refresh token)", status.Code(err))
	}
}

func TestRefreshToken_UnknownToken(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: "does-not-exist"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestRefreshToken_EmptyToken(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestRefreshToken_ExpiredToken(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	store.mu.Lock()
	for hash, rt := range store.refreshTokens {
		rt.ExpiresAt = time.Now().Add(-time.Minute)
		store.refreshTokens[hash] = rt
	}
	store.mu.Unlock()

	_, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestRefreshToken_InactiveUserRejected(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	store.mu.Lock()
	u := store.byID["u1"]
	u.IsActive = false
	store.byID["u1"] = u
	store.mu.Unlock()

	_, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestLogout_RevokesRefreshToken(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	if _, err := s.Logout(context.Background(), &userspb.LogoutRequest{RefreshToken: login.RefreshToken}); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	_, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated after logout", status.Code(err))
	}
}

func TestLogout_UnknownTokenIsIdempotent(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.Logout(context.Background(), &userspb.LogoutRequest{RefreshToken: "does-not-exist"})
	if err != nil {
		t.Errorf("Logout: %v, want nil (idempotent)", err)
	}
}

func TestLogout_EmptyToken(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.Logout(context.Background(), &userspb.LogoutRequest{RefreshToken: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestRefreshToken_ConcurrentSameTokenExactlyOneWins(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			resp, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
				if resp.RefreshToken == "" || resp.AccessToken == "" {
					t.Errorf("winner got an empty token pair: %+v", resp)
				}
			} else if status.Code(err) != codes.Unauthenticated {
				t.Errorf("loser code = %v, want Unauthenticated", status.Code(err))
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Errorf("winners = %d, want exactly 1 (rotation must be atomic)", wins)
	}
}

func TestRefreshToken_ReuseAfterGracePeriodRevokesWholeFamily(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	rotated, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	store.setRevokedAt(login.RefreshToken, time.Now().Add(-2*reuseGracePeriod))
	_, err = s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("replay code = %v, want Unauthenticated", status.Code(err))
	}

	_, err = s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: rotated.RefreshToken})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("family survivor code = %v, want Unauthenticated (whole family must be revoked on reuse)", status.Code(err))
	}
}

func TestRefreshToken_ReuseWithinGracePeriodKeepsFamilyAlive(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	rotated, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	store.setRevokedAt(login.RefreshToken, time.Now())
	_, err = s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("grace-period replay code = %v, want Unauthenticated", status.Code(err))
	}

	if _, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: rotated.RefreshToken}); err != nil {
		t.Errorf("family killed by a grace-period replay: %v, want success", err)
	}
}

func TestRefreshToken_RotatedTokensShareFamily(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store)
	login := loginTestUser(t, s, store)

	resp1, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: login.RefreshToken})
	if err != nil {
		t.Fatalf("first RefreshToken: %v", err)
	}
	resp2, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: resp1.RefreshToken})
	if err != nil {
		t.Fatalf("second RefreshToken: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	families := map[string]bool{}
	for _, rt := range store.refreshTokens {
		families[rt.FamilyID] = true
	}
	if len(families) != 1 {
		t.Errorf("family ids across a login + 2 rotations = %d, want 1 (rotation must continue the family)", len(families))
	}
	if got := store.refreshTokens[auth.HashRefreshToken(resp2.RefreshToken)].UserID; got != "u1" {
		t.Errorf("rotated token user = %q, want u1", got)
	}
}

func registerUser(t *testing.T, s *Server, email, username, password string) *userspb.User {
	t.Helper()
	u, err := s.Register(context.Background(), &userspb.RegisterRequest{Email: email, Username: &username, Password: password})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return u
}

func TestUpdateMe_RequiresMetadata(t *testing.T) {
	s := newTestServer(newFakeStore())
	_, err := s.UpdateMe(context.Background(), &userspb.UpdateMeRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestUpdateMe_ChangesOnlyProvidedFields(t *testing.T) {
	s := newTestServer(newFakeStore())
	u := registerUser(t, s, "a@example.com", "alice", "password123")

	newName := "  alice2 "
	got, err := s.UpdateMe(withUserID(u.Id), &userspb.UpdateMeRequest{Username: &newName})
	if err != nil {
		t.Fatalf("UpdateMe: %v", err)
	}
	if got.GetUsername() != "alice2" || got.GetEmail() != "a@example.com" {
		t.Fatalf("got %+v, want username alice2 and unchanged email", got)
	}

	newEmail := " B@Example.com "
	got, err = s.UpdateMe(withUserID(u.Id), &userspb.UpdateMeRequest{Email: &newEmail})
	if err != nil {
		t.Fatalf("UpdateMe: %v", err)
	}
	if got.GetEmail() != "b@example.com" || got.GetUsername() != "alice2" {
		t.Fatalf("got %+v, want email b@example.com and unchanged username", got)
	}

	if _, err := s.Login(context.Background(), &userspb.LoginRequest{Email: "b@example.com", Password: "password123"}); err != nil {
		t.Fatalf("Login with new email: %v", err)
	}
}

func TestUpdateMe_RejectsInvalidEmail(t *testing.T) {
	s := newTestServer(newFakeStore())
	u := registerUser(t, s, "a@example.com", "alice", "password123")

	bad := "not-an-email"
	_, err := s.UpdateMe(withUserID(u.Id), &userspb.UpdateMeRequest{Email: &bad})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateMe_ConflictsWithOtherAccounts(t *testing.T) {
	s := newTestServer(newFakeStore())
	registerUser(t, s, "a@example.com", "alice", "password123")
	u := registerUser(t, s, "b@example.com", "bob", "password123")

	taken := "a@example.com"
	_, err := s.UpdateMe(withUserID(u.Id), &userspb.UpdateMeRequest{Email: &taken})
	if status.Code(err) != codes.AlreadyExists || status.Convert(err).Message() != "email already registered" {
		t.Fatalf("email conflict: err = %v", err)
	}

	takenName := "ALICE"
	_, err = s.UpdateMe(withUserID(u.Id), &userspb.UpdateMeRequest{Username: &takenName})
	if status.Code(err) != codes.AlreadyExists || status.Convert(err).Message() != "username already taken" {
		t.Fatalf("username conflict: err = %v", err)
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	s := newTestServer(newFakeStore())
	u := registerUser(t, s, "a@example.com", "alice", "password123")

	_, err := s.ChangePassword(withUserID(u.Id), &userspb.ChangePasswordRequest{CurrentPassword: "nope", NewPassword: "newpassword1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestChangePassword_RejectsShortPassword(t *testing.T) {
	s := newTestServer(newFakeStore())
	u := registerUser(t, s, "a@example.com", "alice", "password123")

	_, err := s.ChangePassword(withUserID(u.Id), &userspb.ChangePasswordRequest{CurrentPassword: "password123", NewPassword: "short"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestChangePassword_RotatesCredentialsAndSessions(t *testing.T) {
	s := newTestServer(newFakeStore())
	u := registerUser(t, s, "a@example.com", "alice", "password123")

	old, err := s.Login(context.Background(), &userspb.LoginRequest{Email: "a@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	fresh, err := s.ChangePassword(withUserID(u.Id), &userspb.ChangePasswordRequest{CurrentPassword: "password123", NewPassword: "newpassword1"})
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if fresh.GetAccessToken() == "" || fresh.GetRefreshToken() == "" {
		t.Fatalf("expected a fresh token pair, got %+v", fresh)
	}

	if _, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: old.GetRefreshToken()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old refresh token: code = %v, want Unauthenticated", status.Code(err))
	}
	if _, err := s.RefreshToken(context.Background(), &userspb.RefreshTokenRequest{RefreshToken: fresh.GetRefreshToken()}); err != nil {
		t.Fatalf("fresh refresh token: %v", err)
	}

	if _, err := s.Login(context.Background(), &userspb.LoginRequest{Email: "a@example.com", Password: "password123"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old password: code = %v, want Unauthenticated", status.Code(err))
	}
	if _, err := s.Login(context.Background(), &userspb.LoginRequest{Email: "a@example.com", Password: "newpassword1"}); err != nil {
		t.Fatalf("new password: %v", err)
	}
}

func TestDeleteMe_WrongPassword(t *testing.T) {
	s := newTestServer(newFakeStore())
	u := registerUser(t, s, "a@example.com", "alice", "password123")

	_, err := s.DeleteMe(withUserID(u.Id), &userspb.DeleteMeRequest{Password: "nope"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
	if _, err := s.GetMe(withUserID(u.Id), &emptypb.Empty{}); err != nil {
		t.Fatalf("account must survive a failed delete: %v", err)
	}
}

func TestDeleteMe_RemovesAccount(t *testing.T) {
	s := newTestServer(newFakeStore())
	u := registerUser(t, s, "a@example.com", "alice", "password123")

	if _, err := s.DeleteMe(withUserID(u.Id), &userspb.DeleteMeRequest{Password: "password123"}); err != nil {
		t.Fatalf("DeleteMe: %v", err)
	}
	if _, err := s.GetMe(withUserID(u.Id), &emptypb.Empty{}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetMe after delete: code = %v, want NotFound", status.Code(err))
	}
	if _, err := s.Login(context.Background(), &userspb.LoginRequest{Email: "a@example.com", Password: "password123"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Login after delete: code = %v, want Unauthenticated", status.Code(err))
	}
	registerUser(t, s, "a@example.com", "alice", "password123")
}
