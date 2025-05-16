package log

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
)

func init() {
	logrus.StandardLogger().SetFormatter(&logrus.TextFormatter{PadLevelText: true})
	logrus.StandardLogger().SetOutput(os.Stdout)
}

func G(ctx context.Context) *logrus.Entry {
	return logrus.WithContext(ctx)
}
