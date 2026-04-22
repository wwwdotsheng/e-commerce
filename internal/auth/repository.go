package auth

import (
	"context"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	RepoErrSessionNotFound  = errors.New("session not found in redis")
	RepoErrDatabaseInternal = errors.New("database internal error")
)

var (
	sidContainPrefix = "ident:sess"
	userSidsPrefix   = "ident:u_sess"
)

// 创建一个session
var createSessionScript = redis.NewScript(`
	local sid_contain_key = KEYS[1]
	local user_sid_key = KEYS[2]
	
	local access_token    = ARGV[1]
    local refresh_token   = ARGV[2]
    local expire_seconds  = ARGV[3]
    local session_id      = ARGV[4]

    redis.call("HSET", sid_contain_key, "at", access_token, "rt", refresh_token)
    redis.call("EXPIRE", sid_contain_key, expire_seconds)
	
	redis.call("SADD", user_sid_key, session_id)
	local sids = redis.call("SMEMBERS", user_sid_key)
	if #sids > 5 then
		for _, sid in ipairs(sids) do
			if redis.call("EXISTS", "ident:sess:" .. sid) == 0 then
				redis.call("SREM", user_sid_key, sid)
			end
		end
	end
	return 1
`)

var deleteSessionScript = redis.NewScript(`
	local sid_contain_key = KEYS[1]
	local user_sid_key = KEYS[2]

	local remove_sid = ARGV[1]

	redis.call("DEL", sid_contain_key)
	redis.call("SREM", user_sid_key, remove_sid)
	return 1
`)

var setAccessTokenScript = redis.NewScript(`
	local sid_contain_key = KEYS[1]

	local new_access_token = ARGV[1]
	
	if redis.call("EXISTS", sid_contain_key) == 0 then return 0 end
	redis.call("HSET", sid_contain_key, "at", new_access_token)
	return 1
`)
var setRefreshTokenScript = redis.NewScript(`
	local sid_contain_key = KEYS[1]

	local new_refresh_token = ARGV[1]
	local expire_seconds = ARGV[2]

	if redis.call("EXISTS", sid_contain_key) == 0 then return 0 end
	
	redis.call("HSET", sid_contain_key, "rt", new_refresh_token)
	redis.call("EXPIRE", sid_contain_key, expire_seconds)
	return 1
`)

type Repository struct {
	db     *gorm.DB
	rdb    *redis.Client
	config *config.AuthSection
}

func NewRepository(db *gorm.DB, rdb *redis.Client, config *config.AuthSection) *Repository {
	return &Repository{db: db, rdb: rdb, config: config}
}

func genSidContainKey(sessionId string) string {
	return fmt.Sprintf("%s:%s", sidContainPrefix, sessionId)
}
func genUserSidsKey(accountId uuid.UUID) string {
	return fmt.Sprintf("%s:%d", userSidsPrefix, accountId)
}

// createSession 为用户创建一个session(使用lua脚本)
func (repo *Repository) createSession(ctx context.Context, accountId uuid.UUID, sessionId string, accessToken string, refreshToken string, expireSeconds time.Duration) error {
	cmd := createSessionScript.Run(
		ctx,
		repo.rdb,
		[]string{
			genSidContainKey(sessionId),
			genUserSidsKey(accountId),
		},
		accessToken,
		refreshToken,
		int(expireSeconds.Seconds()),
		sessionId,
	)
	return cmd.Err()
}

func (repo *Repository) getRefreshToken(ctx context.Context, sessionId string) (string, error) {
	cmd := repo.rdb.HGet(ctx, genSidContainKey(sessionId), "rt")
	result, err := cmd.Result()
	return result, err
}

func (repo *Repository) getAccessToken(ctx context.Context, sessionId string) (string, error) {
	cmd := repo.rdb.HGet(ctx, genSidContainKey(sessionId), "at")
	result, err := cmd.Result()
	return result, err
}

func (repo *Repository) FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	result := repo.db.WithContext(ctx).Where("email = ?", email).First(&user)
	return &user, result.Error
}

func (repo *Repository) SetAccessToken(ctx context.Context, accountId uuid.UUID, sessionId string, accessToken string) error {
	cmd := setAccessTokenScript.Run(
		ctx,
		repo.rdb,
		[]string{
			genSidContainKey(sessionId),
		},
		accessToken,
	)
	result, err := cmd.Int()
	if err != nil {
		return fmt.Errorf("%w: %v", RepoErrDatabaseInternal, cmd.Err())
	}
	if result == 0 {
		return RepoErrSessionNotFound
	}
	return nil
}

func (repo *Repository) SetRefreshToken(ctx context.Context, accountId uuid.UUID, sessionId string, refreshToken string) error {
	cmd := setRefreshTokenScript.Run(
		ctx,
		repo.rdb,
		[]string{
			genSidContainKey(sessionId),
		},
		refreshToken,
		repo.config.RefreshTokenExpire,
	)
	result, err := cmd.Int()
	if err != nil {
		return fmt.Errorf("%w: %v", RepoErrDatabaseInternal, cmd.Err())
	}
	if result == 0 {
		return RepoErrSessionNotFound
	}
	return nil
}

func (repo *Repository) DelSession(ctx context.Context, accountId uuid.UUID, sessionId string) error {
	cmd := deleteSessionScript.Run(
		ctx,
		repo.rdb,
		[]string{
			genSidContainKey(sessionId),
			genUserSidsKey(accountId),
		},
		sessionId,
	)
	return cmd.Err()
}
