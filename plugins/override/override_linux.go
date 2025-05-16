package override

import (
	"context"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/plugins"
	"github.com/sirupsen/logrus"
)

var _ plugins.Pluginer = (*override)(nil)

const (
	name = "override"
)

type override struct {
	config *Config
	log    *logrus.Entry
}

func (o *override) Name() string {
	return name
}

func (o *override) Default() plugins.Configer {
	return &Config{}
}

func (o *override) CreateContainer(ctx context.Context, pod *api.PodSandbox, container *api.Container) (*api.ContainerAdjustment, []*api.ContainerUpdate, error) {
	var adjust = &api.ContainerAdjustment{}
	if o.config == nil {
		return adjust, nil, nil
	}
	changed := o.config.Apply(adjust)
	if changed {
		o.log.WithFields(logrus.Fields{
			"container_name": container.Name,
			"adjust":         adjust,
		}).Info("override: container adjustment")
	}
	return adjust, nil, nil
}

func (o *override) Configure(ctx context.Context, config, runtime, version string) (api.EventMask, error) {
	o.log = log.G(ctx).WithField(plugins.FieldName, name)

	o.log.WithFields(logrus.Fields{
		"runtime": runtime,
		"version": version,
	}).Infof("configure %s plugin", name)

	if config != "" {
		_, err := o.config.ReadFrom(strings.NewReader(config))
		if err != nil {
			o.log.WithError(err).Error("failed to load config")
			return 0, err
		}
	}
	return api.EventMask(api.Event_CREATE_CONTAINER), nil
}

func New(cfg *Config) plugins.Pluginer {
	return &override{config: cfg}
}
