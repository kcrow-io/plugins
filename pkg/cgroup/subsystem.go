package cgroup

import "fmt"

type Subsystem string

const (
	Cpu    Subsystem = "cpu"
	Memory Subsystem = "memory"
	Blkio  Subsystem = "blkio"
)

var validSubsystems = map[string]Subsystem{
	"cpu":    Cpu,
	"memory": Memory,
	"blkio":  Blkio,
}

func Valid(sub string) (Subsystem, error) {
	if s, ok := validSubsystems[sub]; ok {
		return s, nil
	}
	return "", fmt.Errorf("invalid subsystem %s", sub)
}

func CgroupSub(sub ...string) ([]Subsystem, error) {

	var result []Subsystem
	added := make(map[Subsystem]bool)
	var hasValid bool

	for _, s := range sub {
		if sub, ok := validSubsystems[s]; ok {
			if !added[sub] {
				result = append(result, sub)
				added[sub] = true
				hasValid = true
			}
		}
	}

	if !hasValid && len(sub) > 0 {
		return nil, fmt.Errorf("no valid subsystems found")
	}

	return result, nil
}
