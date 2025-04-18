package main

import (
	"context"
	"log"
	"net"
	examplev1 "protoc-gen-go-mcp/examples/gen/example/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newVibeServiceClient(ctx context.Context) examplev1.VibeServiceClient {
	// Create a listener on an available port
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Create a new gRPC server and register the vibeServiceServer
	grpcServer := grpc.NewServer()
	vibeServer := &vibeServiceServer{vibe: "initial vibe"}
	examplev1.RegisterVibeServiceServer(grpcServer, vibeServer)

	// Start the server in a goroutine
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// Shutdown the server when context is done
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	// Dial the server using the listener's address
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to dial server: %v", err)
	}

	// Return the new client
	return examplev1.NewVibeServiceClient(conn)
}
