package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"zawaj/internal/models"
)

func TestWriteDownloadSetsAttachmentHeadersBeforeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeDownload(ctx, "application/pdf", `attachment; filename="guests.pdf"`, []byte("pdf"))

	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename="guests.pdf"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := recorder.Body.String(); got != "pdf" {
		t.Fatalf("body = %q", got)
	}
}

func TestExportLanguagePrefersQueryAndFallsBackToHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/?lang=fr", nil)
	ctx.Request.Header.Set("Accept-Language", "ar")
	if got := exportLanguage(ctx); got != "fr" {
		t.Fatalf("exportLanguage with query = %q, want fr", got)
	}

	headerContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	headerContext.Request = httptest.NewRequest("GET", "/", nil)
	headerContext.Request.Header.Set("Accept-Language", "ar-DZ,fr;q=0.8")
	if got := exportLanguage(headerContext); got != "ar" {
		t.Fatalf("exportLanguage with header = %q, want ar", got)
	}
}

func TestGuestCountIncludesTheInvitee(t *testing.T) {
	if got := guestCount(models.Guest{Companions: 2}); got != 3 {
		t.Fatalf("guestCount = %d, want 3", got)
	}
}

func TestTruncatePDFTextFitsTheColumn(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("dejavu", "", dejavuFont)
	pdf.AddPage()
	pdf.SetFont("dejavu", "", 9)
	got := truncatePDFText(pdf, "A very long guest name that must fit", 30)
	if pdf.GetStringWidth(got) > 30 {
		t.Fatalf("truncated name width = %f, want <= 30", pdf.GetStringWidth(got))
	}
}

func TestShapeArabicUsesConnectedPresentationForms(t *testing.T) {
	shaped := shapeArabic("سلام")
	if shaped == "سلام" {
		t.Fatal("Arabic text was not shaped")
	}
	for _, r := range shaped {
		if r >= '\u0600' && r <= '\u06ff' {
			t.Fatalf("shaped Arabic still contains base rune %U in %q", r, shaped)
		}
	}
}
