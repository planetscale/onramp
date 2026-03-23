package serve

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// RandHex generates a random 32-byte hex API key.
func RandHex(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AuthMiddleware returns an Echo middleware that validates Bearer or X-API-Key headers.
func AuthMiddleware(apiKey string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := c.Request().Header.Get("X-API-Key")
			if key == "" {
				auth := c.Request().Header.Get("Authorization")
				key, _ = strings.CutPrefix(auth, "Bearer ")
			}
			if key != apiKey {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			return next(c)
		}
	}
}
