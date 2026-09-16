package auth

import "context"

// Principal 已认证主体。业务项目可把 Login 填成手机号、邮箱或用户名。
type Principal struct {
	UserID int64
	Login  string // 登录标识（手机号 / 用户名 / 邮箱等）
	Token  string
	// Extra 可选扩展（角色码、租户 ID 等）；框架不解释内容。
	Extra map[string]any
}

type ctxKey int

const principalKey ctxKey = 1

// WithPrincipal 将主体写入 context。
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext 读取主体；未登录时 ok=false。
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	if !ok || p.UserID <= 0 {
		return Principal{}, false
	}
	return p, true
}

// UserIDFromContext 便捷读取用户 ID。
func UserIDFromContext(ctx context.Context) (int64, bool) {
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		return 0, false
	}
	return p.UserID, true
}
