package cgroup

type options struct {
	subsystem string
}

type Opt func(*options)

func WithSubsystem(subsystem string) Opt {
	return func(opt *options) {
		opt.subsystem = subsystem
	}
}
