package plugins

import (
	"context"
	"io"

	"github.com/containerd/nri/pkg/stub"
	"github.com/kcrow-io/plugins/pkg/log"
)

const (
	FieldName        = "plugin_name"
	AnnotationPrefix = "io.kcrow."
)

var (
	plugins = make(map[string]Pluginer)
)

type Configer interface {
	ReadFrom(r io.Reader) (int64, error)
	WriteTo(w io.Writer) (int64, error)
}

type Pluginer interface {
	Name() string
	Default() Configer
}

type NopConfig struct{}

func (n *NopConfig) ReadFrom(r io.Reader) (int64, error) {
	return 0, nil
}

func (n *NopConfig) WriteTo(w io.Writer) (int64, error) {
	return 0, nil
}

func RunStub(p Pluginer) error {
	var (
		err error
		ctx = context.Background()
	)

	st, err := stub.New(p)
	if err != nil {
		log.G(ctx).WithField(FieldName, p.Name()).WithError(err).Fatal("failed to create stub")
		return err
	}

	if err = st.Run(ctx); err != nil {
		log.G(ctx).WithField(FieldName, p.Name()).WithError(err).Fatal("failed to run stub")
	}
	return err
}

func RegisterPlugin(p Pluginer) {
	if p != nil {
		plugins[p.Name()] = p
	}
}

func Plugins() map[string]Pluginer {
	return plugins
}
