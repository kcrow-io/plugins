package main

import (
	"os"

	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/plugins"
	"github.com/kcrow-io/plugins/plugins/memory"
)

func main() {
	// Run the NRI stub
	if err := plugins.RunStub(memory.New()); err != nil {
		log.G(nil).WithError(err).Fatal("Failed to run memory plugin")
		os.Exit(1)
	}
}
