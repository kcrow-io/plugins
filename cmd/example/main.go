package main

import (
	"context"
	"os"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
)

type example struct{}

func (o *example) Name() string {
	return "example"
}

func (o *example) CreateContainer(ctx context.Context, pod *api.PodSandbox, container *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
	var adjust = &api.ContainerAdjustment{}
	for _, mount := range container.Mounts {
		if mount != nil && strings.HasPrefix(mount.Destination, "/var/run/secrets") {
			adjust.RemoveMount(mount.Destination)
		}
	}
	return adjust, nil, nil
}

func main() {
	if err := plugins.RunStub(&example{}); err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to run example plugin")
		os.Exit(1)
	}
}
