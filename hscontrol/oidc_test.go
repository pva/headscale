package hscontrol

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/juanfont/headscale/hscontrol/types"
)

func TestSetCSRFCookieSameSite(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/register/test", nil)

	_, err := setCSRFCookie(w, r, "state", false)
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

func TestSetCSRFCookieSecureFromServerURL(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/register/test", nil)
	w := httptest.NewRecorder()

	a := &AuthProviderOIDC{serverURL: "https://headscale.example.com"}
	_, err := setCSRFCookie(w, r, "state", a.cookiesSecure())
	if err != nil {
		t.Fatalf("setCSRFCookie: %v", err)
	}
	if cookie := w.Result().Cookies()[0]; !cookie.Secure {
		t.Fatal("cookie is not Secure for an HTTPS server URL")
	}
}

func TestGetRegistrationIDFromStateSingleUse(t *testing.T) {
	a := &AuthProviderOIDC{
		registrationCache: expirable.NewLRU[string, RegistrationInfo](16, nil, time.Minute),
	}
	id := types.MustRegistrationID()
	a.registrationCache.Add("state", RegistrationInfo{RegistrationID: id})

	if got := a.getRegistrationIDFromState("state"); got == nil || *got != id {
		t.Fatalf("first lookup = %v, want %q", got, id)
	}
	if got := a.getRegistrationIDFromState("state"); got != nil {
		t.Fatalf("second lookup = %q, want nil", *got)
	}
}

func TestClearOIDCCallbackCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearOIDCCallbackCookie(w, "state_abcdef")

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Path != "/oidc/callback" || cookies[0].MaxAge >= 0 {
		t.Fatalf("deletion cookie = %#v", cookies[0])
	}
}
