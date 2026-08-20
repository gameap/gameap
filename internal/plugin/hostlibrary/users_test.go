package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/common"
	"github.com/gameap/gameap/pkg/plugin/sdk/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsersService_FindUsers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.UserRepository)
		request   *users.FindUsersRequest
		wantTotal int
		wantIDs   []uint
	}{
		{
			name: "no_filter_returns_all",
			setupRepo: func(r *inmemory.UserRepository) {
				_ = r.Save(context.Background(), &domain.User{Login: "user1", Email: "user1@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "user2", Email: "user2@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "user3", Email: "user3@test.com"})
			},
			request:   &users.FindUsersRequest{},
			wantTotal: 3,
			wantIDs:   []uint{1, 2, 3},
		},
		{
			name: "filter_by_ids",
			setupRepo: func(r *inmemory.UserRepository) {
				_ = r.Save(context.Background(), &domain.User{Login: "user1", Email: "user1@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "user2", Email: "user2@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "user3", Email: "user3@test.com"})
			},
			request: &users.FindUsersRequest{
				Filter: &users.UserFilter{Ids: []uint64{1, 3}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "filter_by_login",
			setupRepo: func(r *inmemory.UserRepository) {
				_ = r.Save(context.Background(), &domain.User{Login: "admin", Email: "admin@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "user", Email: "user@test.com"})
			},
			request: &users.FindUsersRequest{
				Filter: &users.UserFilter{Login: new("admin")},
			},
			wantTotal: 1,
			wantIDs:   []uint{1},
		},
		{
			name: "filter_by_email",
			setupRepo: func(r *inmemory.UserRepository) {
				_ = r.Save(context.Background(), &domain.User{Login: "user1", Email: "specific@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "user2", Email: "other@test.com"})
			},
			request: &users.FindUsersRequest{
				Filter: &users.UserFilter{Email: new("specific@test.com")},
			},
			wantTotal: 1,
			wantIDs:   []uint{1},
		},
		{
			name: "pagination_applied",
			setupRepo: func(r *inmemory.UserRepository) {
				for i := 1; i <= 10; i++ {
					_ = r.Save(context.Background(), &domain.User{
						Login: "user" + string(rune('0'+i)),
						Email: "user" + string(rune('0'+i)) + "@test.com",
					})
				}
			},
			request: &users.FindUsersRequest{
				Pagination: &common.Pagination{Limit: 3, Offset: 2},
			},
			wantTotal: 3,
			wantIDs:   []uint{3, 4, 5},
		},
		{
			name: "sorting_applied",
			setupRepo: func(r *inmemory.UserRepository) {
				_ = r.Save(context.Background(), &domain.User{Login: "charlie", Email: "c@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "alice", Email: "a@test.com"})
				_ = r.Save(context.Background(), &domain.User{Login: "bob", Email: "b@test.com"})
			},
			request: &users.FindUsersRequest{
				Sorting: []*common.Sorting{{Field: "login", Descending: false}},
			},
			wantTotal: 3,
			wantIDs:   []uint{2, 3, 1},
		},
		{
			name:      "empty_repository_returns_empty",
			setupRepo: func(_ *inmemory.UserRepository) {},
			request:   &users.FindUsersRequest{},
			wantTotal: 0,
			wantIDs:   []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewUserRepository()
			tt.setupRepo(repo)

			svc := NewUsersService(repo)
			resp, err := svc.FindUsers(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, int32(tt.wantTotal), resp.Total)
			require.Len(t, resp.Users, tt.wantTotal)

			for i, wantID := range tt.wantIDs {
				assert.Equal(t, uint64(wantID), resp.Users[i].Id)
			}
		})
	}
}

func TestUsersService_GetUser(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.UserRepository)
		userID    uint64
		wantFound bool
		wantLogin string
	}{
		{
			name: "existing_returns_found",
			setupRepo: func(r *inmemory.UserRepository) {
				_ = r.Save(context.Background(), &domain.User{
					Login: "testuser",
					Email: "test@test.com",
					Name:  new("Test User"),
				})
			},
			userID:    1,
			wantFound: true,
			wantLogin: "testuser",
		},
		{
			name:      "missing_returns_not_found",
			setupRepo: func(_ *inmemory.UserRepository) {},
			userID:    999,
			wantFound: false,
		},
		{
			name: "wrong_id_returns_not_found",
			setupRepo: func(r *inmemory.UserRepository) {
				_ = r.Save(context.Background(), &domain.User{Login: "user1", Email: "user1@test.com"})
			},
			userID:    999,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewUserRepository()
			tt.setupRepo(repo)

			svc := NewUsersService(repo)
			resp, err := svc.GetUser(context.Background(), &users.GetUserRequest{Id: tt.userID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantFound {
				require.NotNil(t, resp.User)
				assert.Equal(t, tt.wantLogin, resp.User.Login)
				assert.Equal(t, tt.userID, resp.User.Id)
			} else {
				assert.Nil(t, resp.User)
			}
		})
	}
}

func TestConvertUserToProto(t *testing.T) {
	t.Parallel()
	user := &domain.User{
		ID:    42,
		Login: "testlogin",
		Email: "test@example.com",
		Name:  new("Test Name"),
	}

	result := convertUserToProto(user)

	assert.Equal(t, uint64(42), result.Id)
	assert.Equal(t, "testlogin", result.Login)
	assert.Equal(t, "test@example.com", result.Email)
	require.NotNil(t, result.Name)
	assert.Equal(t, "Test Name", *result.Name)
}

func TestConvertUserToProto_NilName(t *testing.T) {
	t.Parallel()
	user := &domain.User{
		ID:    1,
		Login: "noname",
		Email: "noname@example.com",
		Name:  nil,
	}

	result := convertUserToProto(user)

	assert.Equal(t, uint64(1), result.Id)
	assert.Nil(t, result.Name)
}

func TestNewUsersHostLibrary(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewUserRepository()
	lib := NewUsersHostLibrary(repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
