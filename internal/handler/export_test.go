package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
