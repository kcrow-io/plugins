package main

import (
	"context"
	"os"

	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/kcrow-io/plugins/pkg/plugins/override"
)

func main() {
	ov := override.New(override.Default())
	if err := plugins.RunStub(ov); err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to run override plugin")
		os.Exit(1)
	}
}
