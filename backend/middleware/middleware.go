package middleware

import (
	"context"
	"net/http"
	"strings"

	"cuti-app/config"
	"cuti-app/utils"
)

type Middleware func(http.HandlerFunc) http.Handler

// Chain applies middlewares in order (first listed = outermost) around a final handler.
func Chain(h http.HandlerFunc, mws ...func(http.Handler) http.Handler) http.Handler {
	var handler http.Handler = h
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}

type contextKey string

const claimsKey contextKey = "claims"

// Auth verifies the Bearer JWT token and stores the parsed claims in the request context.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			utils.Error(w, http.StatusUnauthorized, "token tidak ditemukan, silakan login")
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := utils.ParseToken(tokenStr)
		if err != nil {
			utils.Error(w, http.StatusUnauthorized, "sesi anda telah berakhir, silakan login kembali")
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole only allows requests whose JWT role matches one of the given roles.
// Pass no roles to allow any authenticated user.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool)
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowed) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			claims, ok := GetClaims(r)
			if !ok {
				utils.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !allowed[claims.RoleName] {
				utils.Error(w, http.StatusForbidden, "anda tidak memiliki akses untuk aksi ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GetClaims(r *http.Request) (*utils.Claims, bool) {
	claims, ok := r.Context().Value(claimsKey).(*utils.Claims)
	return claims, ok
}

// CORS allows the frontend to call the API. If FRONTEND_ORIGINS is set
// (comma-separated, e.g. "https://siicup.vercel.app,https://siicup-git-preprod.vercel.app")
// only those origins are allowed; otherwise any origin is allowed, which is
// fine for local development.
func CORS(next http.Handler) http.Handler {
	var allowed map[string]bool
	if config.App.FrontendOrigins != "" {
		allowed = make(map[string]bool)
		for _, o := range strings.Split(config.App.FrontendOrigins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowed[o] = true
			}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		switch {
		case allowed == nil:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case allowed[origin]:
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
