package log

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
)

func init() {
	output := os.Stdout
	logrus.StandardLogger().SetFormatter(&logrus.TextFormatter{PadLevelText: true})
	logrus.StandardLogger().SetOutput(output)
}

func G(ctx context.Context) *logrus.Entry {
	return logrus.WithContext(ctx)
}
