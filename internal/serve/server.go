// Package serve
package serve

import (
	"onramp/internal/hammer"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// NewServer creates and configures an Echo server with all routes registered.
func NewServer(state *hammer.SimulationState, apiKey string, engine *hammer.Engine) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(AuthMiddleware(apiKey))

	h := &Handler{state: state, engine: engine}

	e.GET("/users", h.GetUsers)
	e.GET("/users/current", h.GetCurrentUsers)
	e.POST("/users", h.PostUsers)
	e.GET("/users/flows", h.GetFlows)
	e.POST("/users/flows", h.PostFlows)
	e.POST("/hammer/start", h.HammerStart)
	e.POST("/hammer/stop", h.HammerStop)

	return e
}
