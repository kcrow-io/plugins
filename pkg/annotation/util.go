package annotation

import (
	"strings"

	"github.com/kcrow-io/plugins/plugins"
)

const (
	Separator = ","
)

func IterSuffix(kv map[string]string, fn func(suffix string, class string, value string) (bool, error)) error {
	for k, v := range kv {
		if strings.HasPrefix(k, plugins.AnnotationPrefix) {
			vendorClass := strings.Split(k, "/")
			class := ""
			if len(vendorClass) == 2 {
				class = vendorClass[1]
			}
			next, err := fn(strings.TrimLeft(k, plugins.AnnotationPrefix), class, v)
			if err != nil {
				return err
			}
			if !next {
				return nil
			}
		}
	}
	return nil
}
