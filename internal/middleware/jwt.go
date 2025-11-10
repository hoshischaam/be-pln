package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	response "github.com/hoshichaam/pln_backend_go/pkg/response"
)

func jwtMiddleware(secret string, required bool) fiber.Handler {
	secret = strings.TrimSpace(secret)
	return func(c *fiber.Ctx) error {
		authHeader := strings.TrimSpace(c.Get(fiber.HeaderAuthorization))
		if authHeader == "" {
			if required {
				return response.Error(c, fiber.StatusUnauthorized, "authorization header is required")
			}
			return c.Next()
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			if required {
				return response.Error(c, fiber.StatusUnauthorized, "invalid authorization header")
			}
			return c.Next()
		}

		tokenStr := strings.TrimSpace(parts[1])
		if tokenStr == "" {
			if required {
				return response.Error(c, fiber.StatusUnauthorized, "authorization token is empty")
			}
			return c.Next()
		}

		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid token signing method")
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			if required {
				return response.Error(c, fiber.StatusUnauthorized, "invalid or expired token")
			}
			return c.Next()
		}

		if sub, ok := claims["sub"].(string); ok && sub != "" {
			c.Locals("userId", sub)
		}
		c.Locals("claims", claims)
		return c.Next()
	}
}

func JWTOptional(secret string) fiber.Handler {
	return jwtMiddleware(secret, false)
}

func JWTRequired(secret string) fiber.Handler {
	return jwtMiddleware(secret, true)
}
