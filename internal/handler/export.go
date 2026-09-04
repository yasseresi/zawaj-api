package handler

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"zawaj/internal/auth"
	"zawaj/internal/events"
	"zawaj/internal/middleware"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

// DejaVu Sans includes Arabic glyphs and is embedded in the PDF response so
// deployed API instances do not depend on a host machine's font directory.
//
//go:embed assets/DejaVuSansCondensed.ttf
var dejavuFont []byte

// Export is the CSV export module, mounted under /weddings/:id/export.csv.
// It reuses the guest repo directly (thin passthrough — no service needed).
type Export struct {
	guests   *repository.GuestRepo
	repo     *repository.WeddingRepo // for RequireRole
	tokens   *auth.Manager
	activity events.Recorder
}

// NewExport builds the module.
func NewExport(guests *repository.GuestRepo, repo *repository.WeddingRepo, tokens *auth.Manager, activity events.Recorder) *Export {
	return &Export{guests: guests, repo: repo, tokens: tokens, activity: activity}
}

// Register mounts the export route. Any member (viewer+) may download.
func (h *Export) Register(rg *gin.RouterGroup) {
	rg.GET("/weddings/:id/export.csv",
		middleware.RequireAuth(h.tokens),
		middleware.RequireRole(h.repo, models.RoleViewer),
		h.guestsCSV,
	)
	rg.GET("/weddings/:id/export.xlsx", middleware.RequireAuth(h.tokens), middleware.RequireRole(h.repo, models.RoleViewer), h.guestsXLSX)
	rg.GET("/weddings/:id/export.pdf", middleware.RequireAuth(h.tokens), middleware.RequireRole(h.repo, models.RoleViewer), h.guestsPDF)
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
	h.recordExport(c)
}

func (h *Export) guestsXLSX(c *gin.Context) {
	guests, err := h.guests.List(c.Request.Context(), middleware.WeddingID(c), repository.GuestFilter{Limit: 10000})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "export failed")
		return
	}

	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	const sheet = "Guests"
	headers := []string{"full_name", "contact", "relationship", "status", "companions", "table_label", "meal", "created_at"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = book.SetCellValue(sheet, cell, header)
	}
	for row, guest := range guests {
		values := []any{guest.FullName, guest.Contact, guest.Relationship, string(guest.Status), guest.Companions, guest.TableLabel, guest.Meal, guest.CreatedAt.Format(time.RFC3339)}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row+2)
			_ = book.SetCellValue(sheet, cell, value)
		}
	}
	var out bytes.Buffer
	if err := book.Write(&out); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "export failed")
		return
	}
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", out.Bytes())
	c.Header("Content-Disposition", `attachment; filename="guests.xlsx"`)
	h.recordExport(c)
}

// guestsPDF creates a compact printable guest list. The backend keeps the
// same columns as CSV/XLSX so exports remain interchangeable.
func (h *Export) guestsPDF(c *gin.Context) {
	guests, err := h.guests.List(c.Request.Context(), middleware.WeddingID(c), repository.GuestFilter{Limit: 10000})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "export failed")
		return
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("dejavu", "", dejavuFont)
	pdf.RTL()
	pdf.SetTitle("Zawaj guest list", false)
	pdf.AddPage()
	pdf.SetFont("dejavu", "", 14)
	pdf.CellFormat(0, 10, "Zawaj guest list", "", 1, "L", false, 0, "")
	pdf.SetFont("dejavu", "", 8)
	for _, guest := range guests {
		line := fmt.Sprintf("%s | %s | %s | %s | %d | %s | %s", guest.FullName, guest.Contact, guest.Relationship, guest.Status, guest.Companions, guest.TableLabel, guest.Meal)
		pdf.MultiCell(0, 5, line, "", "L", false)
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "export failed")
		return
	}
	c.Data(http.StatusOK, "application/pdf", out.Bytes())
	c.Header("Content-Disposition", `attachment; filename="guests.pdf"`)
	h.recordExport(c)
}

func (h *Export) recordExport(c *gin.Context) {
	if h.activity == nil {
		return
	}
	actor, ok := middleware.UserID(c)
	if !ok {
		return
	}
	h.activity.Record(c.Request.Context(), events.Activity{
		WeddingID: middleware.WeddingID(c),
		ActorID:   actor,
		Action:    models.ActListExported,
	})
}
