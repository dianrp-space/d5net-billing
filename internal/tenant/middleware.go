package tenant

import (
	"context"
	"net/http"

	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/db"
	"github.com/dianrp/drp-billing/internal/xid"
)

type ctxKey struct{}

type Info struct {
	ID   xid.ID
	Slug string
	Role string
}

func FromContext(ctx context.Context) (Info, bool) {
	v, ok := ctx.Value(ctxKey{}).(Info)
	return v, ok
}

func WithInfo(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, info)
}

func Middleware(tokens *auth.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				claims, err := tokens.ParseToken(authHeader[7:])
				if err == nil && claims.Type == "access" {
					tid, err := xid.Parse(claims.TenantID)
					if err != nil {
						tid = xid.Nil()
					}
					info := Info{ID: tid, Role: claims.Role}
					ctx := WithInfo(r.Context(), info)
					ctx = db.WithTenant(ctx, tid)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r.Context()); !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TenantHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tid := r.Header.Get("X-Tenant-ID"); tid != "" {
			if id, err := xid.Parse(tid); err == nil {
				ctx := db.WithTenant(r.Context(), id)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
