package handler

import (
	"net/http"
	"strconv"

	"zawaj/internal/apperr"
	"zawaj/internal/auth"
	"zawaj/internal/dto"
	"zawaj/internal/middleware"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/internal/service"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Guest is the guests + notes module, mounted under /weddings/:id/guests.
type Guest struct {
	svc      *service.GuestService
	activity *service.ActivityService
	repo     *repository.WeddingRepo // for RequireRole
	tokens   *auth.Manager
}

// NewGuest builds the module.
func NewGuest(svc *service.GuestService, activity *service.ActivityService, repo *repository.WeddingRepo, tokens *auth.Manager) *Guest {
	return &Guest{svc: svc, activity: activity, repo: repo, tokens: tokens}
}

// Register mounts guest routes. All require membership; writes require editor.
func (h *Guest) Register(rg *gin.RouterGroup) {
	viewer := models.RoleViewer
	editor := models.RoleEditor

	g := rg.Group("/weddings/:id/guests", middleware.RequireAuth(h.tokens))
	g.GET("", middleware.RequireRole(h.repo, viewer), h.list)
	g.POST("", middleware.RequireRole(h.repo, editor), h.create)
	g.GET("/:gid", middleware.RequireRole(h.repo, viewer), h.get)
	g.PATCH("/:gid", middleware.RequireRole(h.repo, editor), h.update)
	g.DELETE("/:gid", middleware.RequireRole(h.repo, editor), h.remove)
	g.POST("/:gid/notes", middleware.RequireRole(h.repo, editor), h.addNote)
	g.PATCH("/:gid/status", middleware.RequireRole(h.repo, editor), h.setStatus)
}

func (h *Guest) list(c *gin.Context) {
	page, size := paging(c)
	f := repository.GuestFilter{
		Status: c.Query("status"),
		Query:  c.Query("q"),
		Limit:  size,
		Offset: (page - 1) * size,
	}
	res, err := h.svc.List(c.Request.Context(), middleware.WeddingID(c), f)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, res)
}

func (h *Guest) create(c *gin.Context) {
	actor, _ := middleware.UserID(c)
	var req dto.CreateGuestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	g, err := h.svc.Create(c.Request.Context(), middleware.WeddingID(c), actor, req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, g)
}

func (h *Guest) get(c *gin.Context) {
	gid, ok := guestID(c)
	if !ok {
		return
	}
	detail, err := h.svc.Get(c.Request.Context(), middleware.WeddingID(c), gid)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	history, _ := h.activity.GuestHistory(c.Request.Context(), gid, 50)
	response.JSON(c, http.StatusOK, gin.H{
		"guest": detail.Guest, "notes": detail.Notes, "history": history,
	})
}

func (h *Guest) update(c *gin.Context) {
	gid, ok := guestID(c)
	if !ok {
		return
	}
	actor, _ := middleware.UserID(c)
	var req dto.UpdateGuestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	g, err := h.svc.Update(c.Request.Context(), middleware.WeddingID(c), actor, gid, req)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, g)
}

func (h *Guest) remove(c *gin.Context) {
	gid, ok := guestID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), middleware.WeddingID(c), gid); err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *Guest) addNote(c *gin.Context) {
	gid, ok := guestID(c)
	if !ok {
		return
	}
	actor, _ := middleware.UserID(c)
	var req dto.AddNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	n, err := h.svc.AddNote(c.Request.Context(), middleware.WeddingID(c), actor, gid, req.Body)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, n)
}

func (h *Guest) setStatus(c *gin.Context) {
	gid, ok := guestID(c)
	if !ok {
		return
	}
	actor, _ := middleware.UserID(c)
	var req dto.SetStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.Write(c, apperr.Validation(err.Error()))
		return
	}
	g, err := h.svc.SetStatus(c.Request.Context(), middleware.WeddingID(c), actor, gid, req.Status)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.JSON(c, http.StatusOK, g)
}

// guestID parses the :gid path param, writing a 404 on failure.
func guestID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("gid"))
	if err != nil {
		apperr.Write(c, apperr.NotFound("guest not found"))
		return uuid.Nil, false
	}
	return id, true
}

// paging reads ?page= and ?page_size= with sane defaults/caps.
func paging(c *gin.Context) (page, size int) {
	page, _ = strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	size, _ = strconv.Atoi(c.Query("page_size"))
	if size < 1 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	return page, size
}
