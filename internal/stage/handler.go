package stage

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	models "tf200/todoapp/internal"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Get answers `GET /api/stages`: the user's steps between to do and done, in order.
func (h *Handler) Get(c *gin.Context) {
	userID, _ := primitive.ObjectIDFromHex(c.GetString("user_id"))
	stages, err := h.service.Get(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load the workflow"})
		return
	}
	c.JSON(http.StatusOK, stages)
}

// Put answers `PUT /api/stages` with `{"stages": [...]}`: the whole workflow, in order.
func (h *Handler) Put(c *gin.Context) {
	userID, _ := primitive.ObjectIDFromHex(c.GetString("user_id"))
	var req struct {
		Stages []models.Stage `json:"stages"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	stages, err := h.service.Save(c.Request.Context(), userID, req.Stages)
	if errors.Is(err, ErrInvalid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save the workflow"})
		return
	}
	c.JSON(http.StatusOK, stages)
}
