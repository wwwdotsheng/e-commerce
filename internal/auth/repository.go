package auth

import (
	"context"
	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	rdbNotFoundAccessToken = errors.New("not found access token")
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

var (
	atPrefix         = "identity:at"
	rtPrefix         = "identity:rt"
	sidContainPrefix = "identity:sid"
	userSidsPrefix   = "identity:user:sids"
)

// 这里的
var createSessionScript = redis.NewScript(`
	local rt_key = KEYS[1]
	local at_key = KEYS[2]
	local sid_contain_key = KEYS[3]
	local user_sid_key = KEYS[4]
	
	redis.call('SET', at_key, ARGV[1], 'EX', tonumber(ARGV[2]))
	redis.call('SET', rt_key, ARGV[1], 'EX', tonumber(ARGV[3]))
	
	redis.call('HSET', sid_contain_key, 'token_pair', ARGV[4])
	redis.call('EXPIRE', sid_contain_key, tonumber(ARGV[5]))
	
	redis.call('SADD', user_sid_key, ARGV[6])
	
	return "OK"
`)

type Repository struct {
	rdb    *redis.Client
	config *config.AuthSection
}

func NewRepository(rdb *redis.Client, config *config.AuthSection) *Repository {
	return &Repository{rdb: rdb, config: config}
}

func genAccessTokenKey(accessToken string) string { return fmt.Sprintf("%s%s", atPrefix, accessToken) }
func genRefreshTokenKey(refreshToken string) string {
	return fmt.Sprintf("%s:%s", rtPrefix, refreshToken)
}
func genSidContainKey(sessionId string) string {
	return fmt.Sprintf("%s:%s", sidContainPrefix, sessionId)
}
func genUserSidsKey(accountId uint) string {
	return fmt.Sprintf("%s:%d", userSidsPrefix, accountId)
}

// TODO 考虑迁移到 service 层
func genAccessToken() (string, error)  { return generateRandomString(16) }
func genRefreshToken() (string, error) { return generateRandomString(24) }
func genSessionId() (string, error)    { return generateRandomString(24) }
func generateTokens() (at, rt, sid string, err error) {
	if at, err = genAccessToken(); err != nil {
		return
	}
	if rt, err = genRefreshToken(); err != nil {
		return
	}
	if sid, err = genSessionId(); err != nil {
		return
	}
	return
}

// findAccessToken 查找accessToken
func (repo *Repository) findAccessToken(ctx context.Context, accessToken string) (*identity.AccountInfo, error) {
	atKey := genAccessTokenKey(accessToken)
	val, err := repo.rdb.Get(ctx, atKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, rdbNotFoundAccessToken
		}
		return nil, err
	}

	var accountInfo identity.AccountInfo
	err = json.Unmarshal([]byte(val), &accountInfo)
	if err != nil {
		return nil, err
	}
	return &accountInfo, nil
}

// createSession 为用户创建一个session(使用lua脚本)
func (repo *Repository) createSession(ctx context.Context, accountId uint) (*TokenPair, error) {
	at, rt, sid, err := generateTokens()
	if err != nil {
		return nil, err
	}

	keys := []string{
		genAccessTokenKey(at),
		genRefreshTokenKey(rt),
		genSidContainKey(sid),
		genUserSidsKey(accountId),
	}

	accountInfo := identity.AccountInfo{AccountId: accountId, SessionID: sid}
	accountInfoStr, err := json.Marshal(accountInfo)

	tokenPair := TokenPair{
		AccessToken:  at,
		RefreshToken: rt,
	}
	tokenPairStr, err := json.Marshal(tokenPair)

	cmd := createSessionScript.Run(
		ctx,
		repo.rdb,
		keys,
		// ARGV
		accountInfoStr,
		int(time.Minute/time.Second*30),
		int(time.Hour/time.Second*24*7),
		tokenPairStr,
		int(time.Hour/time.Second*24*7+time.Minute/time.Second*5),
		sid,
	)

	if cmd.Err() != nil {
		return nil, cmd.Err()
	}

	return &tokenPair, nil
}
