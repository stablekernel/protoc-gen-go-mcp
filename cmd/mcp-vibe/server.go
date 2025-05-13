package main

import (
	"context"
	"fmt"
	examplev1 "protoc-gen-go-mcp/examples/gen/example/v1"
)

type vibeServiceServer struct {
	examplev1.UnimplementedVibeServiceServer
	vibe string
}

var _ examplev1.VibeServiceServer = &vibeServiceServer{}

// GetVibe implements examplev1.VibeServiceServer.
func (v *vibeServiceServer) GetVibe(context.Context, *examplev1.GetVibeRequest) (*examplev1.GetVibeResponse, error) {
	return &examplev1.GetVibeResponse{
		Vibe: v.vibe,
		Vibes: []*examplev1.VibeScalar{
			{
				VibeBytes: []byte(v.vibe),
			},
		},
	}, nil
}

// SetVibe implements examplev1.VibeServiceServer.
func (v *vibeServiceServer) SetVibe(_ctx context.Context, req *examplev1.SetVibeRequest) (*examplev1.SetVibeResponse, error) {
	newVibe := req.GetVibe()
	if newVibe == "" {
		return nil, fmt.Errorf("vibe cannot be empty")
	}

	previousVibe := v.vibe
	v.vibe = newVibe

	return &examplev1.SetVibeResponse{
		PreviousVibe: previousVibe,
		Vibe:         newVibe,
	}, nil
}
