package auth

import (
	"context"
	"errors"
	"strings"
)

// ErrUnauthorized 鉴权失败（token 缺失/无效/用户不存在等）。
var ErrUnauthorized = errors.New("unauthorized")

// UnauthorizedError 带对外文案的鉴权错误。
type UnauthorizedError struct {
	Message string
}

func (e *UnauthorizedError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return ErrUnauthorized.Error()
	}
	return e.Message
}

func (e *UnauthorizedError) Unwrap() error { return ErrUnauthorized }

// Unauthorized 构造鉴权失败错误。
func Unauthorized(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "未授权"
	}
	return &UnauthorizedError{Message: message}
}

// Authenticator 由业务项目注入：根据 token 解析出 Principal。
// 典型实现：Redis 查会话 → 数据库查用户。
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Principal, error)
}

// AuthenticatorFunc 函数适配器。
type AuthenticatorFunc func(ctx context.Context, token string) (Principal, error)

func (f AuthenticatorFunc) Authenticate(ctx context.Context, token string) (Principal, error) {
	return f(ctx, token)
}
