package escape

import (
	"context"
	"path"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/kcrow-io/plugins/pkg/annotation"
	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/plugins"
	"github.com/kcrow-io/plugins/plugins/escape/store"
	"github.com/kcrow-io/plugins/plugins/escape/store/disk"
	"github.com/sirupsen/logrus"
)

var _ plugins.Pluginer = (*escape)(nil)

const (
	name      = "escape"
	statusDir = "/run/escape/status"
)

type escape struct {
	log *logrus.Entry
	cfg *Config

	store store.Store
}

func (o *escape) Name() string {
	return name
}

func (o *escape) Default() plugins.Configer {
	return &plugins.NopConfig{}
}

func (o *escape) RemoveContainer(ctx context.Context, _ *api.PodSandbox, ctr *api.Container) error {
	v, err := o.store.Get(ctx, ctr.Id)
	if err != nil {
		// TODO, check it's not found error
		return nil
	}
	defer func() {
		if err == nil {
			o.store.Delete(ctx, ctr.Id)
		}
	}()
	loge := o.log.WithFields(logrus.Fields{"container_name": ctr.Name})
	cg, err := cgroup.LoadCgroup(o.cfg.Root,
		cgroup.WithInner(path.Join(o.cfg.Root, v.Id)),
		cgroup.WithSubsystem((v.Subsystem)))
	if err != nil {
		loge.WithError(err).Info("failed to load cgroup")
		return err
	}
	err = cg.Destory()
	if err != nil {
		loge.WithError(err).Info("failed to remove cgroup")
		return err
	}
	loge.WithFields(logrus.Fields{
		"root": o.cfg.Root,
		"id":   v.Id,
		"sub":  v.Subsystem,
	}).Info("remove cgroup success")
	return nil
}

func (o *escape) StartContainer(ctx context.Context, _ *api.PodSandbox, ctr *api.Container) error {
	loge := o.log.WithFields(logrus.Fields{
		"container_name": ctr.Name,
	})

	if ctr.Linux == nil && ctr.Linux.CgroupsPath == "" {
		loge.Info("not found valid cgroup path")
		return nil
	}

	var (
		subs []string
		all  bool
		err  error
	)

	err = annotation.IterSuffix(ctr.Annotations, func(suffix, _, value string) (bool, error) {
		if suffix == name {
			names := strings.Split(value, annotation.Separator)
			for _, name := range names {
				cname := strings.ToLower(strings.TrimSpace(name))
				if cname == "" {
					continue
				}
				if cname == "all" {
					all = true
					return false, nil
				}
				subs = append(subs, cname)
			}
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		loge.WithError(err).Infof("get subsystem failed")
		return nil
	}
	if len(subs) < 1 && !all {
		loge.Infof("not found valid annotations")
		return nil
	}
	if all {
		subs = nil
	}

	crcg, rerr := cgroup.LoadCgroup(path.Join(o.cfg.Root, ctr.Id))
	ctrcg, cerr := cgroup.LoadCgroup(ctr.Linux.CgroupsPath)
	if cerr != nil || rerr != nil {
		loge.WithFields(logrus.Fields{
			"root_cgroup":      rerr,
			"container_cgroup": cerr,
		}).Infof("load container cgroup error")
		return nil
	}

	ps, err := ctrcg.Proc()
	if err != nil {
		loge.WithError(err).Infof("get container pid failed")
		return nil
	}

	subsystem := v1Subsystem(all, subs, ctrcg)
	loge.WithFields(logrus.Fields{
		"cgroupv2": ctrcg.V2(),
		"pid":      ps,
	}).Infof("escpe with subsystem: %v", subsystem)

	err = crcg.AddProc(ps, subsystem...)
	if err != nil {
		loge.WithError(err).Infof("create new cgroup failed")
		return nil
	}
	err = o.store.Save(ctx, ctr.Id, &store.Process{
		Id:        ctr.Id,
		Subsystem: subsystem,
	})
	if err != nil {
		ctrcg.AddProc(ps, subsystem...)
		loge.WithError(err).Infof("store failed")
		return nil
	}

	loge.WithFields(logrus.Fields{
		"old": ctr.Linux.CgroupsPath,
		"new": path.Join(o.cfg.Root, ctr.Id),
	}).Infof("escape success")
	return nil
}

func (o *escape) Configure(ctx context.Context, config, runtime, version string) (api.EventMask, error) {
	var (
		mask api.EventMask
	)
	mask.Set(api.Event_START_CONTAINER, api.Event_REMOVE_CONTAINER)

	o.log = log.G(ctx).WithField(plugins.FieldName, name)

	o.log.WithFields(logrus.Fields{
		"runtime": runtime,
		"version": version,
	}).Infof("configure plugin, handler event: %s", mask.PrettyString())

	if config != "" {
		_, err := o.cfg.ReadFrom(strings.NewReader(config))
		if err != nil {
			o.log.WithError(err).Error("failed to load config")
			return 0, err
		}
	}
	store, err := disk.New(statusDir)
	if err != nil {
		return 0, err
	}
	o.store = store
	return mask, nil
}

func New(cfg *Config) plugins.Pluginer {
	return &escape{cfg: cfg}
}
