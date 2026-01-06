package main

import (
	"context"
	"os"

	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/kcrow-io/plugins/pkg/plugins/escape"
)

func main() {
	if err := plugins.RunStub(escape.New()); err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to run escape plugin")
		os.Exit(1)
	}
}
