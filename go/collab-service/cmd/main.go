package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50052"
	}
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("collab-service gRPC on :%s", port)
	_ = lis

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("collab-service shutting down")
}
