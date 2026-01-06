package escape

import (
	"context"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/opencontainers/cgroups"
	"github.com/sirupsen/logrus"
)

var (
	_ plugins.Pluginer = (*escape)(nil)

	AnnotateKey = plugins.AnnotationPrefix + name
)

const (
	name = "escape"
)

type escape struct {
	log        *logrus.Entry
	cgroupRoot cgroup.Cgroup

	supportSubs map[string]struct{}
}

func (o *escape) Name() string {
	return name
}

func (o *escape) Default() plugins.Configer {
	return &plugins.NopConfig{}
}

func (o *escape) StartContainer(ctx context.Context, _ *api.PodSandbox, ctr *api.Container) error {
	loge := o.log.WithFields(logrus.Fields{
		"container_name": ctr.Name,
	})

	if ctr.Linux == nil || ctr.Linux.CgroupsPath == "" {
		loge.Info("not found valid cgroup path")
		return nil
	}

	var (
		subs string
		err  error
	)

	if len(ctr.Annotations) == 0 {
		loge.Debugf("no escape annotation")
		return nil
	}

	subs, ok := ctr.Annotations[AnnotateKey]
	if !ok {
		loge.Debugf("no escape annotation")
		return err
	}

	// load container cgroup and get pid
	ctrcg, cerr := cgroup.LoadCgroup(ctr.Linux.CgroupsPath)
	if cerr != nil {
		loge.WithFields(logrus.Fields{
			"container_cgroup": cerr,
		}).Infof("load container cgroup error")
		return nil
	}
	ps, err := ctrcg.Proc()
	if err != nil {
		loge.WithError(err).Infof("get container pid failed")
		return nil
	}

	// escape to root cgroup
	if o.cgroupRoot.V2() {
		err = o.cgroupRoot.AddProc(ps)
		if err != nil {
			loge.WithError(err).Infof("add container pid to root cgroup failed")
			return nil
		}
		loge.WithFields(logrus.Fields{
			"cgroupv2": true,
			"pid":      ps,
		}).Infof("escape to root cgroup (cgroupv2)")
	} else {
		o.cgroupRoot.Subsystem()
		for _, name := range strings.Split(subs, plugins.Separator) {
			cname := strings.ToLower(strings.TrimSpace(name))
			if cname == "" {
				continue
			}
			_, ok := o.supportSubs[cname]
			if !ok {
				loge.WithFields(logrus.Fields{
					"subsystem": cname,
				}).Infof("not support subsystem")
			} else {
				err = o.cgroupRoot.AddProc(ps, cname)
				if err != nil {
					loge.WithError(err).Infof("add container pid to root cgroup failed")
				} else {
					loge.WithFields(logrus.Fields{
						"subsystem": cname,
						"pid":       ps,
					}).Infof("escape to root cgroup (v1)")
				}
			}
		}
	}
	return nil
}

func (o *escape) Configure(ctx context.Context, config, runtime, version string) (api.EventMask, error) {
	var (
		mask api.EventMask
		err  error
	)

	o.log = log.G(ctx).WithField(plugins.FieldName, name)

	o.cgroupRoot, err = cgroup.LoadCgroup("/")
	if err != nil {
		o.log.WithError(err).Errorf("load cgroup failed")
		return mask, nil
	}
	for _, sub := range o.cgroupRoot.Subsystem() {
		o.supportSubs[sub] = struct{}{}
	}
	o.log.WithFields(logrus.Fields{
		"runtime":    runtime,
		"version":    version,
		"cgroupV2":   cgroups.IsCgroup2UnifiedMode(),
		"cgroupRoot": o.cgroupRoot,
	}).Infof("configure plugin, handler event: %s", mask.PrettyString())

	mask.Set(api.Event_START_CONTAINER)
	return mask, nil
}

func New() plugins.Pluginer {
	return &escape{
		supportSubs: make(map[string]struct{}),
	}
}
