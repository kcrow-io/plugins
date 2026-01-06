package main

import (
	"context"
	"os"

	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/kcrow-io/plugins/pkg/plugins/memory"
)

func main() {
	// Run the NRI stub
	if err := plugins.RunStub(memory.New()); err != nil {
		log.G(context.TODO()).WithError(err).Fatal("Failed to run memory plugin")
		os.Exit(1)
	}
}
