package tenant

import (
	"context"
	"net/http"

	"github.com/dianrp-space/d5net-billing/internal/auth"
	"github.com/dianrp-space/d5net-billing/internal/db"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

type ctxKey struct{}

type Info struct {
	ID     xid.ID
	Slug   string
	Role   string
	UserID xid.ID
}

func FromContext(ctx context.Context) (Info, bool) {
	v, ok := ctx.Value(ctxKey{}).(Info)
	return v, ok
}

func WithInfo(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, info)
}

// CanAccessTenantFunc melaporkan apakah user masih boleh memakai token-nya di
// tenant tertentu (aktif + masih terdaftar). Dipakai untuk menolak access token
// milik user yang dinonaktifkan atau dihapus dari tenant, yang token JWT-nya
// masih valid sampai kedaluwarsa.
type CanAccessTenantFunc func(ctx context.Context, userID, tenantID xid.ID) (bool, error)

func Middleware(tokens *auth.TokenService, canAccess CanAccessTenantFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				claims, err := tokens.ParseToken(authHeader[7:])
				if err == nil && claims.Type == "access" {
					tid, terr := xid.Parse(claims.TenantID)
					uid, _ := xid.Parse(claims.UserID)
					if canAccess != nil && !xid.IsNil(uid) {
						if xid.IsNil(tid) || terr != nil {
							// Token tanpa tenant tidak bisa diverifikasi.
							next.ServeHTTP(w, r)
							return
						}
						ok, aerr := canAccess(r.Context(), uid, tid)
						if aerr != nil || !ok {
							// Perlakukan seperti tidak terautentikasi.
							next.ServeHTTP(w, r)
							return
						}
					}
					if terr != nil {
						tid = xid.Nil()
					}
					info := Info{ID: tid, Role: claims.Role, UserID: uid}
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
