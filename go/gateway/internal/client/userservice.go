package client

import (
	"bytes"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func UserServiceURL() string {
	if u := os.Getenv("USER_SERVICE_URL"); u != "" {
		return u
	}
	return "http://localhost:8081"
}

// Proxy repassa a requisição ao user-service preservando método, body e headers relevantes.
func Proxy(c *gin.Context, targetURL string) {
	var body []byte
	if c.Request.Body != nil {
		var err error
		body, err = io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read request body"})
			return
		}
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to build upstream request"})
		return
	}
	req.Header.Set("Content-Type", c.ContentType())
	if auth := c.GetHeader("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream unreachable"})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}
