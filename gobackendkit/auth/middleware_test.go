package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?token=qtok", nil)
	r.Header.Set("Authorization", "Bearer hdr")
	if got := ExtractToken(r); got != "hdr" {
		t.Fatalf("Authorization should win, got %q", got)
	}

	r = httptest.NewRequest(http.MethodGet, "/x?token=qtok", nil)
	if got := ExtractToken(r); got != "qtok" {
		t.Fatalf("query token, got %q", got)
	}

	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("X-Token", "xt")
	if got := ExtractToken(r); got != "xt" {
		t.Fatalf("X-Token, got %q", got)
	}
}

func TestMiddlewareInjectsPrincipal(t *testing.T) {
	m := NewMiddleware(AuthenticatorFunc(func(ctx context.Context, token string) (Principal, error) {
		if token != "good" {
			return Principal{}, Unauthorized("bad token")
		}
		return Principal{UserID: 7, Login: "u7"}, nil
	}))

	var seen int64
	h := m.Handler(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("missing principal")
		}
		seen = p.UserID
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Token", "good")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusOK || seen != 7 {
		t.Fatalf("code=%d seen=%d", rr.Code, seen)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}
