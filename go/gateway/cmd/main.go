package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/britojp/collabdocs/go/gateway/internal/client"
	"github.com/britojp/collabdocs/go/gateway/internal/middleware"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := gin.Default()

	// Rotas públicas — sem JWT
	auth := r.Group("/auth")
	{
		auth.POST("/register", proxyAuth)
		auth.POST("/login", proxyAuth)
	}

	// Rotas protegidas — JWT obrigatório
	protected := r.Group("/", middleware.Auth())
	{
		users := protected.Group("/users")
		{
			users.GET("/:id", proxyUser)
			users.PUT("/:id", proxyUser)
			users.DELETE("/:id", proxyUser)
		}

		docs := protected.Group("/docs")
		{
			docs.GET("", proxyDocService)
			docs.POST("", proxyDocService)
			docs.GET("/:docId", proxyDocService)
			docs.DELETE("/:docId", proxyDocService)
			docs.GET("/:docId/history", proxyDocService)
			docs.GET("/:docId/permissions", proxyDocService)
			docs.PUT("/:docId/permissions/:userId", proxyDocService)
		}

		protected.GET("/ws/docs/:docId", handleWebSocket)
		protected.GET("/sse/admin/metrics", handleSSE)
	}

	log.Printf("gateway starting on :%s", port)
	log.Fatal(r.Run(":" + port))
}

func proxyAuth(c *gin.Context) {
	target := fmt.Sprintf("%s%s", client.UserServiceURL(), c.Request.URL.Path)
	client.Proxy(c, target)
}

func proxyUser(c *gin.Context) {
	target := fmt.Sprintf("%s%s", client.UserServiceURL(), c.Request.URL.Path)
	client.Proxy(c, target)
}

func proxyDocService(c *gin.Context) {
	// TODO: chamar doc-service via gRPC (DOC_SERVICE_ADDR)
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func handleWebSocket(c *gin.Context) {
	// TODO: upgrade para WebSocket, registrar cliente no hub, repassar ops ao doc-service via gRPC
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func handleSSE(c *gin.Context) {
	// TODO: iniciar stream SSE com métricas do Redis
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
