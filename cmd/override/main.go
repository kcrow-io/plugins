package main

import (
	"github.com/kcrow-io/plugins/plugins"
	"github.com/kcrow-io/plugins/plugins/override"
)

func main() {
	ov := override.New(override.Default())
	plugins.RunStub(ov)
}
