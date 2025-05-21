package escape

import (
	"strings"

	"github.com/kcrow-io/plugins/pkg/cgroup"
)

const (
	removeMark = '-'
)

var (
	actualsubsystem = map[string]string{
		"cpu":      "cpu,cpuacct",
		"cpuacct":  "cpu,cpuacct",
		"net_cls":  "net_cls,net_prio",
		"net_prio": "net_cls,net_prio",
	}
)

func actual(sub string) string {
	v, ok := actualsubsystem[sub]
	if ok {
		return v
	}
	return sub
}

func v1Subsystem(all bool, escape []string, origin cgroup.Cgroup) []string {
	var (
		avaliable = origin.Subsystem()
		result    []string
		remove    = map[string]struct{}{}
		add       = map[string]struct{}{}
	)

	if all {
		return avaliable
	}
	for _, v := range escape {
		value := strings.ToLower(strings.TrimSpace(v))
		if value == "" {
			continue
		}
		if value[0] == removeMark {
			remove[value[1:]] = struct{}{}
			remove[actual(value[1:])] = struct{}{}
			continue
		}
		add[value] = struct{}{}
	}
	// not support `remove` and `add` both nil/exist
	if len(remove) == 0 && len(add) == 0 ||
		(len(remove) != 0 && len(add) != 0) {
		return nil
	}

	if len(remove) != 0 {
		for _, v := range avaliable {
			av := actual(v)
			if av != v {
				if _, ok := remove[av]; ok {
					continue
				}
			}
			if _, ok := remove[v]; !ok {
				result = append(result, v)
			}
		}
		return result
	}
	for _, v := range avaliable {
		if _, ok := add[v]; ok {
			result = append(result, v)
		}
	}
	return result
}
