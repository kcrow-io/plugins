package kindenv

import (
	"os/exec"
	"strings"
	"testing"
)

func TestEnsureBinaryMissing(t *testing.T) {
	name := "definitely-missing-binary"
	if _, err := exec.LookPath(name); err == nil {
		t.Skipf("%s unexpectedly present in PATH", name)
	}
	err := ensureBinary(name)
	if err == nil {
		t.Fatalf("expected error when %s binary missing", name)
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("expected error to mention binary name, got %v", err)
	}
}
