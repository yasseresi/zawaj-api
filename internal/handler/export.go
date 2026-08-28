package handler

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"zawaj/internal/auth"
	"zawaj/internal/middleware"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
)

// Export is the CSV export module, mounted under /weddings/:id/export.csv.
// It reuses the guest repo directly (thin passthrough — no service needed).
type Export struct {
	guests *repository.GuestRepo
	repo   *repository.WeddingRepo // for RequireRole
	tokens *auth.Manager
}

// NewExport builds the module.
func NewExport(guests *repository.GuestRepo, repo *repository.WeddingRepo, tokens *auth.Manager) *Export {
	return &Export{guests: guests, repo: repo, tokens: tokens}
}

// Register mounts the export route. Any member (viewer+) may download.
func (h *Export) Register(rg *gin.RouterGroup) {
	rg.GET("/weddings/:id/export.csv",
		middleware.RequireAuth(h.tokens),
		middleware.RequireRole(h.repo, models.RoleViewer),
		h.guestsCSV,
	)
}

// guestsCSV streams all guests of the wedding as a UTF-8 CSV with a BOM so
// Excel renders Arabic text correctly.
//
// guestsCSV godoc
// @Summary  Export guests as CSV
// @Description UTF-8 CSV (with BOM for Excel/Arabic). Any member may download.
// @Tags     export
// @Produce  text/csv
// @Security BearerAuth
// @Param    id path string true "Wedding ID"
// @Success  200 {string} string "CSV file"
// @Failure  403 {object} response.Envelope{error=response.APIError}
// @Router   /weddings/{id}/export.csv [get]
func (h *Export) guestsCSV(c *gin.Context) {
	guests, err := h.guests.List(c.Request.Context(), middleware.WeddingID(c), repository.GuestFilter{
		Limit: 10000,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "export failed")
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="guests.csv"`)

	// UTF-8 BOM: makes Excel detect the encoding and show Arabic correctly.
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(c.Writer)
	defer w.Flush()

	_ = w.Write([]string{
		"full_name", "contact", "relationship", "status",
		"companions", "table_label", "meal", "created_at",
	})

	for _, g := range guests {
		_ = w.Write([]string{
			g.FullName,
			g.Contact,
			g.Relationship,
			string(g.Status),
			strconv.Itoa(g.Companions),
			g.TableLabel,
			g.Meal,
			g.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}
