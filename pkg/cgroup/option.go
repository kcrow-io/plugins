package cgroup

import (
	"path/filepath"

	"github.com/opencontainers/cgroups"
)

const defaultCgroupRoot = "/sys/fs/cgroup"

type options struct {
	subpath []string
	inner   string
}

type Opt func(*options)

// set subsystem array, like cpu, memory, blkio
func WithSubsystem(subpath []string) Opt {
	return func(opt *options) {
		opt.subpath = subpath
	}
}

// set innerpath, could be absolute or relative
// like /system.slice or system.slice
func WithInner(inner string) Opt {
	return func(opt *options) {
		opt.inner = inner
	}
}

func (o *options) subsystem() (map[string]string, error) {
	if len(o.subpath) == 0 || o.inner == "" {
		return nil, nil
	}
	var (
		subsys = make(map[string]string)
	)
	for _, s := range o.subpath {
		sub, err := subsysPath(o.inner, s)
		if err != nil {
			return nil, err
		}
		subsys[s] = sub
	}
	return subsys, nil
}

func subsysPath(inner, subsystem string) (string, error) {
	root := defaultCgroupRoot

	// If the cgroup name/path is absolute do not look relative to the cgroup of the init process.
	if filepath.IsAbs(inner) {
		mnt, err := cgroups.FindCgroupMountpoint(root, subsystem)
		// If we didn't mount the subsystem, there is no point we make the path.
		if err != nil {
			return "", err
		}

		// Sometimes subsystems can be mounted together as 'cpu,cpuacct'.
		return filepath.Join(root, filepath.Base(mnt), inner), nil
	}

	// Use GetOwnCgroupPath for dind-like cases, when cgroupns is not
	// available. This is ugly.
	parentPath, err := cgroups.GetOwnCgroupPath(subsystem)
	if err != nil {
		return "", err
	}

	return filepath.Join(parentPath, inner), nil
}
