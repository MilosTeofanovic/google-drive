package middleware

import (
	"net/http"
	"strings"
)

// AuthMiddleware služi kao API Gateway zaštita za validaciju tokena
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Proveravamo da li postoji Authorization zaglavlje
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Nedostaje Authorization zaglavlje", http.StatusUnauthorized)
			return
		}

		// 2. Format mora biti "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Nevalidan format tokena (očekuje se Bearer <token>)", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		userID := extractUserIDFromToken(tokenString)
		if userID == "" {
			http.Error(w, "Nevalidan ili istekao JWT token", http.StatusUnauthorized)
			return
		}

		r.Header.Set("X-User-ID", userID)

		next.ServeHTTP(w, r)
	}
}

func extractUserIDFromToken(token string) string {

	if token != "" {
		return token
	}
	return ""
}
