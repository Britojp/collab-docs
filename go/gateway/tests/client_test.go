package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/britojp/collabdocs/go/gateway/internal/client"
)

func TestProxy_ForwardsStatusAndBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"token":"abc"}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"a@b.com","password":"pass"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	client.Proxy(c, upstream.URL+"/auth/login")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body, _ := io.ReadAll(w.Body)
	if !strings.Contains(string(body), "token") {
		t.Fatalf("expected body with token, got: %s", string(body))
	}
}

func TestProxy_ForwardsAuthorizationHeader(t *testing.T) {
	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/users/1", nil)
	c.Request.Header.Set("Authorization", "Bearer mytoken")

	client.Proxy(c, upstream.URL+"/users/1")

	if receivedAuth != "Bearer mytoken" {
		t.Fatalf("expected Authorization header to be forwarded, got: %q", receivedAuth)
	}
}

func TestProxy_UpstreamUnreachable(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/users/1", nil)

	client.Proxy(c, "http://localhost:19999/users/1")

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
}

func TestProxy_ForwardsUpstreamErrorCode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/users/1", nil)

	client.Proxy(c, upstream.URL+"/users/1")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 from upstream, got %d", w.Code)
	}
}
