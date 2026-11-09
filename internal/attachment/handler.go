package attachment

import (
	"errors"
	"log"
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

type startRequest struct {
	Name string `json:"name" binding:"required"`
	Size int64  `json:"size" binding:"required"`
	Mime string `json:"mime"`
}

// ListFor answers GET /tasks/:id/attachments or /notes/:id/attachments.
func (h *Handler) ListFor(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, parentID, ok := ids(c)
		if !ok {
			return
		}
		list, err := h.service.List(c.Request.Context(), userID, kind, parentID)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(http.StatusOK, list)
	}
}

// StartFor answers POST /tasks/:id/attachments or /notes/:id/attachments: a pending
// attachment and the link to upload its bytes to.
func (h *Handler) StartFor(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, parentID, ok := ids(c)
		if !ok {
			return
		}
		var req startRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		a, upload, err := h.service.Start(c.Request.Context(), userID, kind, parentID, req.Name, req.Size, req.Mime)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"attachment": a, "upload": upload})
	}
}

// Complete answers POST /attachments/:id/complete, once the upload is done.
func (h *Handler) Complete(c *gin.Context) {
	userID, id, ok := ids(c)
	if !ok {
		return
	}
	a, err := h.service.Complete(c.Request.Context(), userID, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, a)
}

// Download answers GET /attachments/:id/download with a short-lived link to the file.
func (h *Handler) Download(c *gin.Context) {
	userID, id, ok := ids(c)
	if !ok {
		return
	}
	d, err := h.service.Download(c.Request.Context(), userID, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) Delete(c *gin.Context) {
	userID, id, ok := ids(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), userID, id); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func ids(c *gin.Context) (userID, id primitive.ObjectID, ok bool) {
	userID, _ = primitive.ObjectIDFromHex(c.GetString("user_id"))
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return userID, id, false
	}
	return userID, id, true
}

// fail answers with a status, a readable message and a stable code the apps can match on.
func fail(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, "server"
	switch {
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, ErrDisabled):
		status, code = http.StatusServiceUnavailable, "disabled"
	case errors.Is(err, ErrTooLarge):
		status, code = http.StatusRequestEntityTooLarge, "too_large"
	case errors.Is(err, ErrQuota):
		status, code = http.StatusRequestEntityTooLarge, "quota"
	case errors.Is(err, ErrTooMany):
		status, code = http.StatusConflict, "too_many"
	case errors.Is(err, ErrNotUploaded):
		status, code = http.StatusConflict, "not_uploaded"
	case errors.Is(err, ErrSizeChanged):
		status, code = http.StatusBadRequest, "size_changed"
	case errors.Is(err, ErrEmpty), errors.Is(err, ErrBadName), errors.Is(err, ErrBadMime):
		status, code = http.StatusBadRequest, "bad_input"
	}
	if status == http.StatusInternalServerError {
		log.Printf("attachments: %v", err)
		c.JSON(status, gin.H{"error": "something went wrong", "code": code})
		return
	}
	c.JSON(status, gin.H{"error": err.Error(), "code": code})
}
