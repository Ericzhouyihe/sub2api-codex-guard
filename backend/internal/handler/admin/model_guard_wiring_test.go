package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var modelGuardEndpoints = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/admin/codex-eval/state"},
	{http.MethodPost, "/api/v1/admin/codex-eval/start"},
	{http.MethodPost, "/api/v1/admin/codex-eval/cancel"},
	{http.MethodGet, "/api/v1/admin/codex-eval/records/candy/7/example"},
	{http.MethodPost, "/api/v1/admin/codex-eval/clear-history"},
	{http.MethodGet, "/api/v1/admin/codex-eval/inspection/state"},
	{http.MethodPost, "/api/v1/admin/codex-eval/inspection/start"},
	{http.MethodPost, "/api/v1/admin/codex-eval/inspection/stop"},
	{http.MethodPost, "/api/v1/admin/accounts/7/codex-eval"},
	{http.MethodGet, "/api/v1/admin/accounts/7/codex-eval/models"},
	{http.MethodPut, "/api/v1/admin/accounts/7/codex-eval-inspection"},
	{http.MethodGet, "/api/v1/admin/settings/codex-eval-inspection"},
	{http.MethodPut, "/api/v1/admin/settings/codex-eval-inspection"},
}

func newModelGuardWiringRouter(auth middleware.AdminAuthMiddleware) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &handler.Handlers{Admin: &handler.AdminHandlers{
		Account: &adminhandler.AccountHandler{},
		Setting: &adminhandler.SettingHandler{},
	}}
	routes.RegisterAdminRoutes(r.Group("/api/v1"), h, auth,
		middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }),
		middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
	// These routes deliberately expose only the embedded static application.
	r.GET("/codex-eval", h.Admin.Account.CodexEvalPage)
	r.GET("/codex-eval/", h.Admin.Account.CodexEvalPage)
	return r
}

func TestModelGuardAPIsRequireAdminAuthentication(t *testing.T) {
	// Exercise the real auth middleware: a query-string token must not replace
	// a Bearer header or an administrator API key.
	r := newModelGuardWiringRouter(middleware.NewAdminAuthMiddleware(nil, nil, nil, nil))
	for _, endpoint := range modelGuardEndpoints {
		t.Run(endpoint.method+endpoint.path, func(t *testing.T) {
			for _, suffix := range []string{"", "?token=legacy-jwt"} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(endpoint.method, endpoint.path+suffix, strings.NewReader(`{}`)))
				require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			}
		})
	}
	// The static endpoint is not an unauthenticated alternative API namespace.
	for _, path := range []string{"/codex-eval/state", "/codex-eval/inspection/state"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, w.Code)
	}
}

func TestModelGuardAPIsRejectObserverScope(t *testing.T) {
	r := newModelGuardWiringRouter(middleware.AdminAuthMiddleware(func(c *gin.Context) {
		c.Request = c.Request.WithContext(service.WithObserverScope(c.Request.Context(), []int64{10}))
		c.Next()
	}))
	for _, endpoint := range modelGuardEndpoints {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(`{}`)))
		require.Equal(t, http.StatusForbidden, w.Code, endpoint.path+": "+w.Body.String())
	}
}

func TestModelGuardPageIsStaticAndSameOriginOnly(t *testing.T) {
	r := newModelGuardWiringRouter(middleware.NewAdminAuthMiddleware(nil, nil, nil, nil))
	var page string
	for _, path := range []string{"/codex-eval", "/codex-eval/?token=untrusted-query-token"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Header().Get("Content-Type"), "text/html")
		require.Equal(t, "SAMEORIGIN", w.Header().Get("X-Frame-Options"))
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		require.Contains(t, w.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'")
		require.Contains(t, w.Header().Get("Content-Security-Policy"), "connect-src 'self'")
		require.NotContains(t, w.Body.String(), "untrusted-query-token")
		if page == "" {
			page = w.Body.String()
		} else {
			require.Equal(t, page, w.Body.String(), "the public page must not depend on request credentials")
		}
	}
}

func TestModelGuardTransportErrorPreservesCauseAndAttemptMarker(t *testing.T) {
	err := &service.HTTPUpstreamRequestError{Err: context.DeadlineExceeded}
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, context.DeadlineExceeded.Error(), err.Error())
	var attempted interface{ RequestAttempted() bool }
	require.True(t, errors.As(err, &attempted))
	require.True(t, attempted.RequestAttempted())
}
