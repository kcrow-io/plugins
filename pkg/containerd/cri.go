package containerd

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CRIClient wraps the CRI runtime service client
type CRIClient struct {
	conn          *grpc.ClientConn
	runtimeClient runtimeapi.RuntimeServiceClient
}

// NewCRIClient creates a new CRI client connected to the specified socket
func NewCRIClient(socket string) (*CRIClient, error) {
	conn, err := grpc.NewClient(
		socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CRI socket: %w", err)
	}

	return &CRIClient{
		conn:          conn,
		runtimeClient: runtimeapi.NewRuntimeServiceClient(conn),
	}, nil
}

// Close closes the CRI client connection
func (c *CRIClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ListContainers lists all containers using CRI API
func (c *CRIClient) ListContainers(ctx context.Context, filter *runtimeapi.ContainerFilter) ([]*runtimeapi.Container, error) {
	req := &runtimeapi.ListContainersRequest{
		Filter: filter,
	}

	resp, err := c.runtimeClient.ListContainers(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	return resp.Containers, nil
}

// GetContainerStatus gets the status of a specific container
func (c *CRIClient) GetContainerStatus(ctx context.Context, containerID string) (*runtimeapi.ContainerStatus, error) {
	req := &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	}

	resp, err := c.runtimeClient.ContainerStatus(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get container status: %w", err)
	}

	return resp.Status, nil
}
