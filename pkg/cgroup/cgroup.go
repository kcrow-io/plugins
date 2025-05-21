package cgroup

import (
	"context"

	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/fs"
	"github.com/opencontainers/cgroups/fs2"
)

var (
	_ Cgroup = (*manager)(nil)
)

type Cgroup interface {
	Proc() ([]int, error)
	AddProc(pid []int, subs ...string) error
	Subsystem() []string
	Destory() error
	V2() bool
}

type manager struct {
	cg     cgroups.Manager
	iscgv2 bool
}

func (m *manager) Destory() error {
	return m.cg.Destroy()
}

func (m *manager) Proc() ([]int, error) {
	return m.cg.GetPids()
}

func (m *manager) Subsystem() []string {
	var (
		ret []string
	)

	pat := m.cg.GetPaths()
	for k := range pat {
		if k != "" {
			ret = append(ret, k)
		}
	}
	return ret
}

func (m *manager) AddProc(pid []int, subsystems ...string) error {
	var (
		cgmg = m.cg
	)
	// only cgroup v1 and subsystem not nil
	// new cgroup manager
	if !m.iscgv2 && len(subsystems) != 0 {
		var applysubs = make(map[string]string)
		for k, v := range m.cg.GetPaths() {
			for _, sub := range subsystems {
				if k == sub {
					applysubs[k] = v
				}
			}
		}
		cg, err := cgmg.GetCgroups()
		if err != nil {
			return err
		}
		log.G(context.Background()).Infof("apply subsystems: %v", applysubs)
		cgmg, err = fs.NewManager(cg, applysubs)
		if err != nil {
			return err
		}
	}

	for _, p := range pid {
		if err := cgmg.Apply(p); err != nil {
			return err
		}
	}
	return nil
}

func (m *manager) V2() bool {
	return m.iscgv2
}

func LoadCgroup(p string, op ...Opt) (Cgroup, error) {
	var (
		opts = &options{}
	)
	for _, o := range op {
		o(opts)
	}
	cg := cgroups.Cgroup{Path: p, Resources: &cgroups.Resources{}}
	if cgroups.IsCgroup2UnifiedMode() {
		cg2, err := fs2.NewManager(&cg, "")
		if err != nil {
			return nil, err
		}
		return &manager{cg: cg2, iscgv2: true}, nil
	}
	submap, err := opts.subsystem()
	if err != nil {
		return nil, err
	}
	cg1, err := fs.NewManager(&cg, submap)
	if err != nil {
		return nil, err
	}
	return &manager{cg: cg1}, nil
}
