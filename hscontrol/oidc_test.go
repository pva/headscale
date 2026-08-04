package hscontrol

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetCSRFCookieSameSite(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/register/test", nil)

	_, err := setCSRFCookie(w, r, "state")
	if err != nil {
		t.Fatalf("setCSRFCookie: %v", err)
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if got := cookies[0].SameSite; got != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, want Lax", got)
	}
}
