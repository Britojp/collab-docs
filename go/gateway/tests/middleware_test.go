package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/britojp/collabdocs/go/gateway/internal/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter() *gin.Engine {
	r := gin.New()
	r.GET("/protected", middleware.Auth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"userId": c.GetString(middleware.UserIDKey)})
	})
	return r
}

func makeToken(secret string, sub string, exp time.Time) string {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": sub,
		"exp": exp.Unix(),
	})
	tok, _ := t.SignedString([]byte(secret))
	return tok
}

func TestAuth_ValidToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	token := makeToken("test-secret", "user-123", time.Now().Add(time.Hour))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	newRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuth_MissingToken(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)

	newRouter().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_InvalidSignature(t *testing.T) {
	t.Setenv("JWT_SECRET", "correct-secret")

	token := makeToken("wrong-secret", "user-123", time.Now().Add(time.Hour))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	newRouter().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_ExpiredToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	token := makeToken("test-secret", "user-123", time.Now().Add(-time.Hour))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	newRouter().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_MalformedHeader(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Token abc123")

	newRouter().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
