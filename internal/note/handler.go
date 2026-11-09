package note

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Bodies are capped well above anything typed by hand, to keep documents small.
type createRequest struct {
	Title string `json:"title" binding:"max=200"`
	Body  string `json:"body" binding:"max=200000"`
}

type updateRequest struct {
	Title  *string `json:"title" binding:"omitempty,max=200"`
	Body   *string `json:"body" binding:"omitempty,max=200000"`
	Pinned *bool   `json:"pinned"`
}

func (h *Handler) Create(c *gin.Context) {
	userID, _ := primitive.ObjectIDFromHex(c.GetString("user_id"))
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	note, err := h.service.Create(c.Request.Context(), userID, req.Title, req.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create note"})
		return
	}
	c.JSON(http.StatusCreated, note)
}

func (h *Handler) GetAll(c *gin.Context) {
	userID, _ := primitive.ObjectIDFromHex(c.GetString("user_id"))
	notes, err := h.service.GetByUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load notes"})
		return
	}
	c.JSON(http.StatusOK, notes)
}

func (h *Handler) GetByID(c *gin.Context) {
	userID, _ := primitive.ObjectIDFromHex(c.GetString("user_id"))
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	note, err := h.service.GetByID(c.Request.Context(), userID, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, note)
}

func (h *Handler) Update(c *gin.Context) {
	userID, _ := primitive.ObjectIDFromHex(c.GetString("user_id"))
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	note, err := h.service.Update(c.Request.Context(), userID, id, req.Title, req.Body, req.Pinned)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, note)
}

func (h *Handler) Delete(c *gin.Context) {
	userID, _ := primitive.ObjectIDFromHex(c.GetString("user_id"))
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.service.Delete(c.Request.Context(), userID, id); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func fail(c *gin.Context, err error) {
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
}
