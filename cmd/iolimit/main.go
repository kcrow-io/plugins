package main

import (
	"context"
	"os"

	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	ioplugin "github.com/kcrow-io/plugins/pkg/plugins/iolimit"
)

func main() {
	plugin, err := ioplugin.New()
	if err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to create iolimit plugin")
		os.Exit(1)
	}
	if err := plugins.RunStub(plugin); err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to run iolimit plugin")
		os.Exit(1)
	}
}
