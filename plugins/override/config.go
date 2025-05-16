package override

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/containerd/nri/pkg/api"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/plugins"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var _ plugins.Configer = (*Config)(nil)

type Config struct {
	*specs.Spec
}

var rlimitMap = map[string]struct{}{
	"RLIMIT_CPU":        {},
	"RLIMIT_FSIZE":      {},
	"RLIMIT_DATA":       {},
	"RLIMIT_STACK":      {},
	"RLIMIT_CORE":       {},
	"RLIMIT_RSS":        {},
	"RLIMIT_NPROC":      {},
	"RLIMIT_NOFILE":     {},
	"RLIMIT_MEMLOCK":    {},
	"RLIMIT_AS":         {},
	"RLIMIT_LOCKS":      {},
	"RLIMIT_SIGPENDING": {},
	"RLIMIT_MSGQUEUE":   {},
	"RLIMIT_NICE":       {},
	"RLIMIT_RTPRIO":     {},
	"RLIMIT_RTTIME":     {},
}

func (c *Config) ReadFrom(r io.Reader) (int64, error) {
	var newc = &specs.Spec{}
	err := json.NewDecoder(r).Decode(newc)
	if err != nil {
		return 0, err
	}
	c.Spec = newc
	return 0, nil
}

func (c *Config) WriteTo(r io.Writer) (int64, error) {
	encode := json.NewEncoder(r)
	encode.SetIndent("", "  ")
	return 0, encode.Encode(c)
}

func (c *Config) String() string {
	var (
		buf = &strings.Builder{}
	)
	err := json.NewEncoder(buf).Encode(c)
	if err != nil {
		return fmt.Sprintf(`parse config error: %s`, err.Error())
	}
	return buf.String()
}

// apply: process(env, oom_score_adj, rlimits), hook,
func (c *Config) Apply(adj *api.ContainerAdjustment) bool {
	var (
		changed bool
		proc    = c.Spec.Process
	)
	if proc != nil {
		if proc.Env != nil {
			changed = true
			adj.Env = api.FromOCIEnv(proc.Env)
		}
		if proc.Rlimits != nil {
			changed = true
			adj.Rlimits = filterOCIRlimits(proc.Rlimits)
		}
		if proc.OOMScoreAdj != nil {
			changed = true
			adj.Linux = &api.LinuxContainerAdjustment{
				OomScoreAdj: &api.OptionalInt{Value: int64(*proc.OOMScoreAdj)},
			}
		}
	}

	if c.Spec.Hooks != nil {
		changed = true
		adj.Hooks = api.FromOCIHooks(c.Spec.Hooks)
	}
	return changed
}

func Default() *Config {
	return &Config{Spec: &specs.Spec{Version: ""}}
}

func filterOCIRlimits(rlimits []specs.POSIXRlimit) []*api.POSIXRlimit {
	var ret []*api.POSIXRlimit
	for _, r := range rlimits {
		if _, ok := rlimitMap[r.Type]; !ok {
			log.G(context.Background()).WithField("type", r.Type).Warnf("Unsupported rlimit type: %s", r.Type)
			continue
		}
		ret = append(ret, &api.POSIXRlimit{
			Type: r.Type,
			Hard: r.Hard,
			Soft: r.Soft,
		})
	}
	return ret
}
