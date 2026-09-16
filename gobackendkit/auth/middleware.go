package auth

import (
	"encoding/json"
	"errors"
	"net/http"
)

// UnauthorizedWriter 写出 401 响应；业务可注入自己的 JSON 格式。
type UnauthorizedWriter func(w http.ResponseWriter, message string)

// AfterAuthFunc 鉴权成功后的钩子（例如同步旧版 context key）。
type AfterAuthFunc func(r *http.Request, p Principal) *http.Request

// Middleware 可注入鉴权的 HTTP 中间件。
type Middleware struct {
	Auth   Authenticator
	Write  UnauthorizedWriter
	After  AfterAuthFunc
}

// NewMiddleware 创建中间件。auth 必填。
func NewMiddleware(authenticator Authenticator, opts ...Option) *Middleware {
	m := &Middleware{
		Auth:  authenticator,
		Write: defaultUnauthorizedWriter,
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.Auth == nil {
		panic("gobackendkit/auth: Authenticator is required")
	}
	return m
}

// Option 中间件选项。
type Option func(*Middleware)

// WithUnauthorizedWriter 自定义 401 响应格式。
func WithUnauthorizedWriter(w UnauthorizedWriter) Option {
	return func(m *Middleware) {
		if w != nil {
			m.Write = w
		}
	}
}

// WithAfterAuth 鉴权成功后改写 request（常用于兼容旧 context）。
func WithAfterAuth(fn AfterAuthFunc) Option {
	return func(m *Middleware) {
		m.After = fn
	}
}

// Handler 包装业务 HandlerFunc。
func (m *Middleware) Handler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ExtractToken(r)
		if token == "" {
			m.Write(w, "未提供认证token")
			return
		}
		p, err := m.Auth.Authenticate(r.Context(), token)
		if err != nil {
			msg := "token无效或已过期"
			var ue *UnauthorizedError
			if errors.As(err, &ue) && ue.Message != "" {
				msg = ue.Message
			}
			m.Write(w, msg)
			return
		}
		if p.UserID <= 0 {
			m.Write(w, "token无效或用户不存在")
			return
		}
		if p.Token == "" {
			p.Token = token
		}
		ctx := WithPrincipal(r.Context(), p)
		r = r.WithContext(ctx)
		if m.After != nil {
			if nr := m.After(r, p); nr != nil {
				r = nr
			}
		}
		next(w, r)
	}
}

func defaultUnauthorizedWriter(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      false,
		"message": message,
	})
}
