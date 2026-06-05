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
		port = "50051"
	}
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("doc-service gRPC on :%s", port)
	_ = lis

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("doc-service shutting down")
}
