package auth

import (
	"context"
	"crypto/sha256"
	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"encoding/hex"
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

var errTokenExpired = errors.New("token expired")

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "ident:opaque:" + hex.EncodeToString(sum[:])
}

// Session mutations and token indexes must change atomically.
var createSessionScript = redis.NewScript(`
local now = tonumber(redis.call('TIME')[1])
redis.call('HSET', KEYS[1], 'at', ARGV[1], 'rt', ARGV[2], 'account', ARGV[5], 'at_exp', now + tonumber(ARGV[6]), 'rt_exp', now + tonumber(ARGV[3]))
redis.call('EXPIRE', KEYS[1], ARGV[3])
redis.call('SADD', KEYS[2], ARGV[4])
redis.call('SET', KEYS[3], ARGV[4], 'EX', ARGV[3])
redis.call('SET', KEYS[4], ARGV[4], 'EX', ARGV[3])
return 1
`)
var rotateOpaqueScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
local now = tonumber(redis.call('TIME')[1])
local old = redis.call('HGET', KEYS[1], ARGV[1])
if old ~= ARGV[7] then return 0 end
local ttl = redis.call('TTL', KEYS[1])
if ttl <= 0 then return 0 end
if ARGV[1] == 'rt' then ttl = tonumber(ARGV[4]) end
redis.call('DEL', ARGV[5])
redis.call('HSET', KEYS[1], ARGV[1], ARGV[2], ARGV[1] .. '_exp', now + tonumber(ARGV[3]))
redis.call('SET', KEYS[2], ARGV[6], 'EX', ttl)
if ARGV[1] == 'rt' then redis.call('EXPIRE', KEYS[1], ttl) end
return 1
`)
var deleteSessionScript = redis.NewScript(`
redis.call('DEL', KEYS[1], KEYS[3], KEYS[4])
redis.call('SREM', KEYS[2], ARGV[1])
return 1
`)
var verifyOpaqueScript = redis.NewScript(`
local sid = redis.call('GET', KEYS[1])
if not sid then return {} end
local key = 'ident:sess:' .. sid
if redis.call('HGET', key, ARGV[1]) ~= ARGV[2] then return {} end
local exp = redis.call('HGET', key, ARGV[1] .. '_exp')
if not exp then return {} end
if tonumber(exp) <= tonumber(redis.call('TIME')[1]) then return {'expired'} end
return {redis.call('HGET', key, 'account'), sid}
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
	return fmt.Sprintf("%s:%s", userSidsPrefix, accountId)
}

// createSession 为用户创建一个session(使用lua脚本)
func (repo *Repository) createSession(ctx context.Context, accountId uuid.UUID, sessionId string, accessToken string, refreshToken string, expireSeconds time.Duration) error {
	cmd := createSessionScript.Run(
		ctx,
		repo.rdb,
		[]string{
			genSidContainKey(sessionId),
			genUserSidsKey(accountId),
			tokenKey(accessToken), tokenKey(refreshToken),
		},
		accessToken,
		refreshToken,
		int(expireSeconds.Seconds()),
		sessionId, accountId.String(), int(repo.config.AccessTokenExpire.Seconds()),
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

func (repo *Repository) SetAccessToken(ctx context.Context, accountId uuid.UUID, sessionId string, token string) error {
	return repo.rotateToken(ctx, sessionId, "at", token, repo.config.AccessTokenExpire)
}
func (repo *Repository) SetRefreshToken(ctx context.Context, accountId uuid.UUID, sessionId string, token string) error {
	return repo.rotateToken(ctx, sessionId, "rt", token, repo.config.RefreshTokenExpire)
}
func (repo *Repository) rotateToken(ctx context.Context, sid, field, token string, lifetime time.Duration) error {
	old, err := repo.rdb.HGet(ctx, genSidContainKey(sid), field).Result()
	if errors.Is(err, redis.Nil) {
		return RepoErrSessionNotFound
	}
	if err != nil {
		return err
	}
	// Compare the old token inside the transaction to avoid leaving a concurrent rotation index behind.
	result, err := rotateOpaqueScript.Run(ctx, repo.rdb, []string{genSidContainKey(sid), tokenKey(token)}, field, token, int(lifetime.Seconds()), int(repo.config.RefreshTokenExpire.Seconds()), tokenKey(old), sid, old).Int()
	if err != nil {
		return err
	}
	if result == 0 {
		return RepoErrSessionNotFound
	}
	return nil
}
func (repo *Repository) DelSession(ctx context.Context, accountId uuid.UUID, sid string) error {
	tokens, err := repo.rdb.HMGet(ctx, genSidContainKey(sid), "at", "rt").Result()
	if err != nil {
		return err
	}
	at, rt := "", ""
	if tokens[0] != nil {
		at = tokens[0].(string)
	}
	if tokens[1] != nil {
		rt = tokens[1].(string)
	}
	return deleteSessionScript.Run(ctx, repo.rdb, []string{genSidContainKey(sid), genUserSidsKey(accountId), tokenKey(at), tokenKey(rt)}, sid).Err()
}
func (repo *Repository) verifyOpaqueToken(ctx context.Context, token string, kind TokenType) (*identity.AccountInfo, error) {
	field := "at"
	if kind == RefreshToken {
		field = "rt"
	}
	values, err := verifyOpaqueScript.Run(ctx, repo.rdb, []string{tokenKey(token)}, field, token).StringSlice()
	if err != nil {
		return nil, err
	}
	if len(values) == 1 && values[0] == "expired" {
		return nil, errTokenExpired
	}
	if len(values) != 2 {
		return nil, RepoErrSessionNotFound
	}
	id, err := uuid.Parse(values[0])
	if err != nil {
		return nil, err
	}
	return &identity.AccountInfo{AccountId: id, SessionID: values[1]}, nil
}

func (repo *Repository) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	err := repo.db.WithContext(ctx).First(&user, "id = ?", id).Error
	return &user, err
}
