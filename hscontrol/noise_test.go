package hscontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/juanfont/headscale/hscontrol/capver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

// newNoiseRouterWithBodyLimit builds a Gorilla router with the same body-limit
// middleware used by the real Noise router and captures the handler's read.
func newNoiseRouterWithBodyLimit(readBody *[]byte, readErr *error) http.Handler {
	router := mux.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, noiseBodyLimit)
			next.ServeHTTP(w, r)
		})
	})

	handler := func(w http.ResponseWriter, r *http.Request) {
		*readBody, *readErr = io.ReadAll(r.Body)
		if *readErr != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}

		w.WriteHeader(http.StatusOK)
	}

	router.HandleFunc("/machine/map", handler).Methods(http.MethodPost)
	router.HandleFunc("/machine/register", handler).Methods(http.MethodPost)

	return router
}

func TestNoiseBodyLimit_MapEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("normal_map_request", func(t *testing.T) {
		t.Parallel()

		var body []byte
		var readErr error
		router := newNoiseRouterWithBodyLimit(&body, &readErr)

		payload, err := json.Marshal(tailcfg.MapRequest{Version: 100, Stream: true})
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/machine/map", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.NoError(t, readErr)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Len(t, body, len(payload))
	})

	t.Run("oversized_body_rejected", func(t *testing.T) {
		t.Parallel()

		var body []byte
		var readErr error
		router := newNoiseRouterWithBodyLimit(&body, &readErr)

		oversized := bytes.Repeat([]byte("x"), int(noiseBodyLimit)+1)
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/machine/map", bytes.NewReader(oversized))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Error(t, readErr)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		assert.LessOrEqual(t, len(body), int(noiseBodyLimit))
	})
}

func TestNoiseBodyLimit_RegisterEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("normal_register_request", func(t *testing.T) {
		t.Parallel()

		var body []byte
		var readErr error
		router := newNoiseRouterWithBodyLimit(&body, &readErr)

		payload, err := json.Marshal(tailcfg.RegisterRequest{Version: 100})
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/machine/register", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.NoError(t, readErr)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Len(t, body, len(payload))
	})

	t.Run("oversized_body_rejected", func(t *testing.T) {
		t.Parallel()

		var body []byte
		var readErr error
		router := newNoiseRouterWithBodyLimit(&body, &readErr)

		oversized := bytes.Repeat([]byte("x"), int(noiseBodyLimit)+1)
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/machine/register", bytes.NewReader(oversized))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Error(t, readErr)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		assert.LessOrEqual(t, len(body), int(noiseBodyLimit))
	})
}

func TestNoiseBodyLimit_AtExactLimit(t *testing.T) {
	t.Parallel()

	var body []byte
	var readErr error
	router := newNoiseRouterWithBodyLimit(&body, &readErr)

	payload := bytes.Repeat([]byte("a"), int(noiseBodyLimit))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/machine/map", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.NoError(t, readErr)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, body, int(noiseBodyLimit))
}

func TestEarlyNoiseDefersUnsupportedClientRejection(t *testing.T) {
	t.Parallel()

	ns := &noiseServer{challenge: key.NewChallenge()}

	tests := []struct {
		name        string
		version     int
		wantPayload bool
	}{
		{
			name:        "before EarlyNoise support",
			version:     earlyNoiseCapabilityVersion - 1,
			wantPayload: false,
		},
		{
			name:        "unsupported by Headscale but supports EarlyNoise",
			version:     63,
			wantPayload: true,
		},
		{
			name:        "supported by Headscale",
			version:     int(capver.MinSupportedCapabilityVersion),
			wantPayload: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var payload bytes.Buffer
			err := ns.earlyNoise(test.version, &payload)
			require.NoError(t, err)

			if test.wantPayload {
				assert.NotEmpty(t, payload.Bytes())
				assert.True(t, bytes.HasPrefix(payload.Bytes(), []byte(earlyPayloadMagic)))
			} else {
				assert.Empty(t, payload.Bytes())
			}
		})
	}
}

func TestNoiseClientIdentity(t *testing.T) {
	app := createTestApp(t)
	user := app.state.CreateUserForTest("diagnostic-user")
	node := app.state.CreateNodeForTest(user, "old-client")
	node.User = *user

	_, _, err := app.state.SaveNode(node.View())
	require.NoError(t, err)

	identity, ok := app.noiseClientIdentity(node.MachineKey)
	require.True(t, ok)
	assert.Equal(t, node.ID, identity.nodeID)
	assert.Equal(t, "old-client", identity.hostname)
	assert.Equal(t, user.ID, identity.userID)
	assert.Equal(t, user.Username(), identity.username)

	_, ok = app.noiseClientIdentity(key.NewMachine().Public())
	assert.False(t, ok)
}

func TestNoisePollNetMapHandler_OversizedBody(t *testing.T) {
	t.Parallel()

	ns := &noiseServer{}
	oversized := bytes.Repeat([]byte("x"), int(noiseBodyLimit)+1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/machine/map", bytes.NewReader(oversized))
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, noiseBodyLimit)

	ns.NoisePollNetMapHandler(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestNoiseRegistrationHandler_OversizedBody(t *testing.T) {
	t.Parallel()

	ns := &noiseServer{}
	oversized := bytes.Repeat([]byte("x"), int(noiseBodyLimit)+1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/machine/register", bytes.NewReader(oversized))
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, noiseBodyLimit)

	ns.NoiseRegistrationHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
