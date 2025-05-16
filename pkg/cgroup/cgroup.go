package cgroup

import (
	"fmt"

	cgroups "github.com/containerd/cgroups/v3"
	cgroup1 "github.com/containerd/cgroups/v3/cgroup1"
	cgroup2 "github.com/containerd/cgroups/v3/cgroup2"
)

type Cgroup interface {
	Proc() ([]uint64, error)
	AddProc(pid []uint64, subs ...Subsystem) error
	V2() bool
}

var (
	_ Cgroup = (*manager)(nil)
)

type manager struct {
	v1 cgroup1.Cgroup
	v2 *cgroup2.Manager
}

func (m *manager) Proc() ([]uint64, error) {
	if m.v2 != nil {
		return m.v2.Procs(false)
	}
	procs, err := m.v1.Processes(cgroup1.Cpu, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get processes: %w", err)
	}
	var ret = make([]uint64, 0, len(procs))
	for i, p := range procs {
		ret[i] = uint64(p.Pid)
	}
	return ret, nil
}

func (m *manager) AddProc(pid []uint64, subs ...Subsystem) error {
	if m.v2 != nil {
		for _, p := range pid {
			// TODO, should recover when error?
			if err := m.v2.AddProc(p); err != nil {
				return err
			}
		}
		return nil
	}
	var (
		names = []cgroup1.Name{}
	)
	for _, sub := range subs {
		switch sub {
		case Cpu:
			names = append(names, cgroup1.Cpu)
		case Memory:
			names = append(names, cgroup1.Memory)
		case Blkio:
			names = append(names, cgroup1.Blkio)
		}
	}
	for _, p := range pid {
		if err := m.v1.AddProc(p, names...); err != nil {
			return err
		}
	}
	return nil
}

func (m *manager) V2() bool {
	return m.v2 != nil
}

func LoadCgroup(path string, op ...Opt) (Cgroup, error) {
	var (
		opts = &options{}
	)
	for _, o := range op {
		o(opts)
	}

	if cgroups.Mode() == cgroups.Unified {
		cg2, err := cgroup2.Load(path)
		if err != nil {
			return nil, err
		}
		return &manager{v2: cg2}, nil
	}

	cg1, err := cgroup1.Load(cgroup1.NestedPath(path))
	if err != nil {
		return nil, err
	}
	return &manager{v1: cg1}, nil
}
