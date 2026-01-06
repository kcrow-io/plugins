package main

import (
	"context"
	"os"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
)

type vruntime struct {
}

func newVruntime() plugins.Pluginer {
	return &vruntime{}
}

func (o *vruntime) Name() string {
	return "vruntime"
}

func (o *vruntime) Default() plugins.Configer {
	return &plugins.NopConfig{}
}

func (o *vruntime) CreateContainer(ctx context.Context, pod *api.PodSandbox, container *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
	var adjust = &api.ContainerAdjustment{}
	for _, mount := range container.Mounts {
		if mount != nil && strings.HasPrefix(mount.Destination, "/var/run/secrets") {
			adjust.RemoveMount(mount.Destination)
		}
	}
	return adjust, nil, nil
}

func main() {
	plugin := newVruntime()
	if err := plugins.RunStub(plugin); err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to run vruntime plugin")
		os.Exit(1)
	}
}