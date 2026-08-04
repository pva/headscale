package hscontrol

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleVerifyRequest_OversizedBodyRejected(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", int(verifyBodyLimit)+128)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/verify",
		bytes.NewReader([]byte(body)),
	)
	req.Body = http.MaxBytesReader(rec, req.Body, verifyBodyLimit)

	h := &Headscale{}

	err := h.handleVerifyRequest(req, &bytes.Buffer{})
	if err == nil {
		t.Fatal("oversized verify body must be rejected")
	}

	var httpErr HTTPError
	requireHTTPError := errors.As(err, &httpErr)
	if !requireHTTPError {
		t.Fatalf("error must be an HTTPError, got: %T (%v)", err, err)
	}

	assert.Equal(t, http.StatusRequestEntityTooLarge, httpErr.Code)
}
