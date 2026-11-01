package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"tf200/todoapp/internal/auth"
)

type apiTokenFinder interface {
	FindActiveAPITokenByHash(ctx context.Context, tokenHash string) (*auth.APIToken, error)
}

func AuthRequired(jwtSecret string, tokenFinder apiTokenFinder) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		}

		rawToken := parts[1]
		token, err := jwt.Parse(rawToken, func(t *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})
		if err == nil && token.Valid {
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}

			userID, ok := claims["sub"].(string)
			if !ok || userID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}

			c.Set("user_id", userID)
			c.Set("credential_type", "jwt")
			c.Next()
			return
		}

		apiToken, err := tokenFinder.FindActiveAPITokenByHash(c.Request.Context(), auth.HashAPIToken(rawToken))
		if err != nil || apiToken.UserID == primitive.NilObjectID {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set("user_id", apiToken.UserID.Hex())
		c.Set("credential_type", "api_token")
		c.Next()
	}
}

func RequireJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("credential_type") != "jwt" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "login token required"})
			return
		}
		c.Next()
	}
}
