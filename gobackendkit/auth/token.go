package auth

import (
	"net/http"
	"strings"
)

// ExtractToken 从请求提取 token，顺序：
//  1. Authorization: Bearer <token>
//  2. Query ?token=
//  3. Header X-Token
func ExtractToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	token := strings.TrimSpace(r.Header.Get("Authorization"))
	if token != "" {
		token = strings.TrimPrefix(token, "Bearer ")
		token = strings.TrimPrefix(token, "bearer ")
		token = strings.TrimSpace(token)
		if token != "" {
			return token
		}
	}
	if q := strings.TrimSpace(r.URL.Query().Get("token")); q != "" {
		return q
	}
	return strings.TrimSpace(r.Header.Get("X-Token"))
}
