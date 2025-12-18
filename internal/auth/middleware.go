package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const CtxUserIDKey = "user_id"
const CtxUsernameKey = "username"

func RequireJWT(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		tokenStr := strings.TrimPrefix(h, "Bearer ")
		claims, err := ParseJWT(secret, tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(CtxUserIDKey, claims.UserID)
		c.Set(CtxUsernameKey, claims.Username)
		c.Next()
	}
}
