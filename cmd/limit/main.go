package main

import (
	"context"
	"os"

	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	limitplugin "github.com/kcrow-io/plugins/pkg/plugins/limit"
)

func main() {
	if err := plugins.RunStub(limitplugin.New()); err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to run limit plugin")
		os.Exit(1)
	}
}
