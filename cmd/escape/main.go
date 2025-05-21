package main

import (
	"github.com/kcrow-io/plugins/plugins"
	"github.com/kcrow-io/plugins/plugins/escape"
)

func main() {
	plugins.RunStub(escape.New(escape.DefaultConfig()))
}
