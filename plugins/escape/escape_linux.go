package escape

import (
	"context"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
	"github.com/kcrow-io/plugins/pkg/annotation"
	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/plugins"
	"github.com/sirupsen/logrus"
)

var _ plugins.Pluginer = (*escape)(nil)

const (
	name      = "escape"
	k8scgroup = "kubepods"
)

type escape struct {
	s   stub.Stub
	log *logrus.Entry
}

func (o *escape) Name() string {
	return name
}

func (o *escape) Default() plugins.Configer {
	return &plugins.NopConfig{}
}

func (o *escape) PostStartContainer(ctx context.Context, pod *api.PodSandbox, ctr *api.Container) error {
	if ctr.Linux == nil && ctr.Linux.CgroupsPath == "" {
		return nil
	}
	cgpaths := strings.Split(ctr.Linux.CgroupsPath, "/")
	if len(cgpaths) < 1 || !strings.HasPrefix(cgpaths[0], k8scgroup) {
		o.log.WithFields(logrus.Fields{
			"cgroupath":      ctr.Linux.CgroupsPath,
			"container_name": ctr.Name,
		}).Infof("cgroup path not match kubepods, ignore")
		return nil
	}
	var (
		subs []cgroup.Subsystem
		err  error
	)
	loge := o.log.WithFields(logrus.Fields{
		"container_name": ctr.Name,
		"cgroupath":      ctr.Linux.CgroupsPath,
	})
	annotation.IterSuffix(ctr.Annotations, func(suffix, _, value string) (bool, error) {
		if suffix == name {
			names := strings.Split(value, annotation.Separator)
			subs, err = cgroup.CgroupSub(names...)
			return false, err
		}
		return true, nil
	})
	if err != nil {
		o.log.WithError(err).Infof("get cgroup sub error")
		return nil
	}
	if len(subs) < 1 {
		return nil
	}
	kubecg, kerr := cgroup.LoadCgroup("/" + cgpaths[0])
	ctrcg, cerr := cgroup.LoadCgroup(ctr.Linux.CgroupsPath)
	if kerr != nil || cerr != nil {
		loge.Infof("load cgroup error")
		return nil
	}
	ps, err := ctrcg.Proc()
	if err != nil {
		loge.Infof("get container proc failed")
		return nil
	}
	err = kubecg.AddProc(ps, subs...)
	if err != nil {
		loge.Infof("add container proc to kubepods failed")
		return nil
	}
	loge.Infof("escape done")
	return nil
}

func (o *escape) Configure(ctx context.Context, _, runtime, version string) (api.EventMask, error) {
	log.G(ctx).WithFields(logrus.Fields{
		"runtime": runtime,
		"version": version,
	}).Infof("configretion '%s' plugin done", name)
	o.log = log.G(ctx).WithField(plugins.FieldName, name)

	return api.EventMask(api.Event_POST_START_CONTAINER), nil
}

func New() plugins.Pluginer {
	return &escape{}
}
