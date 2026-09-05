package handler

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"zawaj/internal/auth"
	"zawaj/internal/events"
	"zawaj/internal/middleware"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/go-typeset/bidi"
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
	writeDownload(c, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", `attachment; filename="guests.xlsx"`, out.Bytes())
	h.recordExport(c)
}

// guestsPDF creates a printable, localized guest table.
func (h *Export) guestsPDF(c *gin.Context) {
	guests, err := h.guests.List(c.Request.Context(), middleware.WeddingID(c), repository.GuestFilter{Limit: 10000})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "export failed")
		return
	}
	language := exportLanguage(c)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("dejavu", "", dejavuFont)
	pdf.SetTitle(pdfString(language, "قائمة المدعوين", "Guest list", "Liste des invités"), false)
	pdf.AddPage()
	drawPDFHeader(pdf, language)
	drawPDFGuestTable(pdf, guests, language)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeInternal, "export failed")
		return
	}
	writeDownload(c, "application/pdf", `attachment; filename="guests.pdf"`, out.Bytes())
	h.recordExport(c)
}

func exportLanguage(c *gin.Context) string {
	language := c.Query("lang")
	if language == "" {
		language = strings.Split(c.GetHeader("Accept-Language"), ",")[0]
	}
	language = strings.ToLower(strings.TrimSpace(strings.Split(language, "-")[0]))
	switch language {
	case "ar", "fr":
		return language
	default:
		return "en"
	}
}

func pdfString(language, arabic, english, french string) string {
	switch language {
	case "ar":
		return arabic
	case "fr":
		return french
	default:
		return english
	}
}

func drawPDFHeader(pdf *fpdf.Fpdf, language string) {
	pdf.SetFont("dejavu", "", 16)
	pdf.SetTextColor(55, 40, 36)
	title := pdfString(language, "قائمة المدعوين", "Guest list", "Liste des invités")
	if language == "ar" {
		title = shapeArabic(title)
	}
	pdf.CellFormat(0, 10, title, "", 1, "C", false, 0, "")
	pdf.SetFont("dejavu", "", 9)
	pdf.SetTextColor(100, 80, 74)
	subtitle := pdfString(language, "الاسم وعدد الأشخاص وحالة الدعوة", "Name, guest count and RSVP status", "Nom, nombre d'invités et statut")
	if language == "ar" {
		subtitle = shapeArabic(subtitle)
	}
	pdf.CellFormat(0, 7, subtitle, "", 1, "C", false, 0, "")
	pdf.Ln(7)
}

func drawPDFGuestTable(pdf *fpdf.Fpdf, guests []models.Guest, language string) {
	margin := 14.0
	pageWidth, _ := pdf.GetPageSize()
	tableWidth := pageWidth - (margin * 2)
	columnWidths := []float64{tableWidth * 0.60, tableWidth * 0.20, tableWidth * 0.20}
	headers := []string{
		pdfString(language, "الاسم", "Name", "Nom"),
		pdfString(language, "عدد الأشخاص", "Guests", "Invités"),
		pdfString(language, "الحالة", "Status", "Statut"),
	}
	if language == "ar" {
		columnWidths = []float64{columnWidths[2], columnWidths[1], columnWidths[0]}
		headers = []string{headers[2], headers[1], headers[0]}
	}

	drawPDFTableHeader(pdf, margin, columnWidths, headers, language)

	for row, guest := range guests {
		if pdf.GetY() > 270 {
			pdf.AddPage()
		}

		fill := row%2 == 0
		if fill {
			pdf.SetFillColor(250, 242, 239)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		pdf.SetTextColor(55, 40, 36)
		pdf.SetFont("dejavu", "", 9)
		name := guest.FullName
		if language == "ar" {
			name = shapeArabic(name)
		}
		name = truncatePDFText(pdf, name, columnWidths[0]-4)
		count := strconv.Itoa(guestCount(guest))

		status := ""
		if guest.Status == models.StatusConfirmed {
			status = "✓"
			pdf.SetTextColor(40, 125, 65)
		} else if guest.Status == models.StatusDeclined {
			status = "✕"
			pdf.SetTextColor(180, 55, 50)
		}
		if language == "ar" {
			pdf.CellFormat(columnWidths[0], 10, status, "1", 0, "C", fill, 0, "")
			pdf.SetTextColor(55, 40, 36)
			pdf.CellFormat(columnWidths[1], 10, count, "1", 0, "C", fill, 0, "")
			pdf.CellFormat(columnWidths[2], 10, name, "1", 0, "R", fill, 0, "")
		} else {
			pdf.SetTextColor(55, 40, 36)
			pdf.CellFormat(columnWidths[0], 10, name, "1", 0, "L", fill, 0, "")
			pdf.CellFormat(columnWidths[1], 10, count, "1", 0, "C", fill, 0, "")
			if status != "" {
				if guest.Status == models.StatusConfirmed {
					pdf.SetTextColor(40, 125, 65)
				} else {
					pdf.SetTextColor(180, 55, 50)
				}
			}
			pdf.CellFormat(columnWidths[2], 10, status, "1", 0, "C", fill, 0, "")
		}
		pdf.Ln(-1)
	}
}

func drawPDFTableHeader(pdf *fpdf.Fpdf, margin float64, columns []float64, headers []string, language string) {
	pdf.SetX(margin)
	pdf.SetFont("dejavu", "", 10)
	pdf.SetFillColor(157, 84, 67)
	pdf.SetTextColor(255, 255, 255)
	for i, header := range headers {
		text := header
		if language == "ar" {
			text = shapeArabic(text)
		}
		pdf.CellFormat(columns[i], 11, text, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
}

func truncatePDFText(pdf *fpdf.Fpdf, text string, width float64) string {
	if pdf.GetStringWidth(text) <= width {
		return text
	}
	const suffix = "..."
	runes := []rune(text)
	for len(runes) > 0 {
		candidate := string(runes) + suffix
		if pdf.GetStringWidth(candidate) <= width {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return suffix
}

func guestCount(guest models.Guest) int {
	return guest.Companions + 1
}

func writeDownload(c *gin.Context, contentType, disposition string, body []byte) {
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", disposition)
	c.Data(http.StatusOK, contentType, body)
}

// shapeArabic converts Arabic joining forms to presentation forms, then puts
// the paragraph in visual order for fpdf's left-to-right text writer. Latin
// usernames and separators remain readable in the mixed guest-list line.
func shapeArabic(text string) string {
	runes := []rune(text)
	forms := bidi.JoinForms(runes)
	for i, r := range runes {
		runes[i] = bidi.PresentationForm(r, forms[i])
	}
	return bidi.VisualOrder(string(runes), bidi.RightToLeft)
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
