//go:build tools

package main

import (
	// https://golangci-lint.run
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	// https://go.dev/blog/vuln
	_ "golang.org/x/vuln/cmd/govulncheck"
)
