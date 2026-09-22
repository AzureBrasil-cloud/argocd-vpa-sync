package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testDashboardFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>dashboard shell</html>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
		"favicon.svg":   {Data: []byte("<svg/>")},
	}
}

func TestSPAHandler_ServesIndexAtRoot(t *testing.T) {
	h := spaHandler(testDashboardFS())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>dashboard shell</html>" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestSPAHandler_ServesRealStaticAsset(t *testing.T) {
	h := spaHandler(testDashboardFS())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "console.log('app')" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestSPAHandler_FallsBackToIndexForClientSideRoute(t *testing.T) {
	h := spaHandler(testDashboardFS())
	rec := httptest.NewRecorder()
	// A deep-linked client-side route (e.g. after a page refresh) has no
	// matching file in the build output; the SPA's own router must take
	// over once index.html loads.
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/recommendations/payments/checkout-api-vpa/app", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (index.html fallback), got %d", rec.Code)
	}
	if rec.Body.String() != "<html>dashboard shell</html>" {
		t.Fatalf("expected the index.html shell, got: %s", rec.Body.String())
	}
}
