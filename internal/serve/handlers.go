package serve

import (
	"net/http"

	"onramp/internal/hammer"
	"onramp/internal/hammer/htypes"

	"github.com/labstack/echo/v4"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	state  *hammer.SimulationState
	engine *hammer.Engine
}

// GetUsers returns the current UserConfig as JSON.
func (h *Handler) GetUsers(c echo.Context) error {
	return c.JSON(http.StatusOK, h.state.GetUserConfig())
}

// PostUsers updates the UserConfig from the JSON body.
func (h *Handler) PostUsers(c echo.Context) error {
	var uc htypes.UserConfig
	if err := c.Bind(&uc); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if uc.MaxUsers < uc.MinUsers {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "max_users must be >= min_users"})
	}
	h.state.SetUserConfig(uc)
	return c.JSON(http.StatusOK, uc)
}

// GetFlows returns the current flows as JSON.
func (h *Handler) GetFlows(c echo.Context) error {
	return c.JSON(http.StatusOK, h.state.GetFlows())
}

// GetCurrentUsers returns the current flows as JSON.
func (h *Handler) GetCurrentUsers(c echo.Context) error {
	return c.JSON(http.StatusOK, h.state.GetUsers())
}

// PostFlows replaces the entire flows slice from the JSON body.
func (h *Handler) PostFlows(c echo.Context) error {
	var flows []htypes.Flow
	if err := c.Bind(&flows); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	h.state.SetFlows(flows)
	return c.JSON(http.StatusOK, flows)
}

// HammerStart starts the simulation engine.
func (h *Handler) HammerStart(c echo.Context) error {
	if h.state.IsRunning() {
		return c.JSON(http.StatusConflict, map[string]string{"error": "simulation already running"})
	}
	h.state.SetRunning(true)
	h.engine.Start()
	return c.JSON(http.StatusOK, map[string]string{"status": "started"})
}

// HammerStop stops the simulation engine.
func (h *Handler) HammerStop(c echo.Context) error {
	if !h.state.IsRunning() {
		return c.JSON(http.StatusConflict, map[string]string{"error": "simulation not running"})
	}
	h.state.SetRunning(false)
	h.engine.Stop()
	return c.JSON(http.StatusOK, map[string]string{"status": "stopped"})
}
