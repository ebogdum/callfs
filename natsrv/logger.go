package natsrv

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

type zapLogger struct {
	logger *zap.Logger
}

func (l zapLogger) Noticef(format string, v ...any) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

func (l zapLogger) Warnf(format string, v ...any) {
	l.logger.Warn(fmt.Sprintf(format, v...))
}

func (l zapLogger) Fatalf(format string, v ...any) {
	l.logger.Error(fmt.Sprintf(format, v...))
}

func (l zapLogger) Errorf(format string, v ...any) {
	message := fmt.Sprintf(format, v...)
	if strings.HasPrefix(message, routeRetryPrefix) {
		l.logger.Warn(message)
		return
	}
	l.logger.Error(message)
}

const routeRetryPrefix = "Error trying to connect to route"

func (l zapLogger) Debugf(format string, v ...any) {
	l.logger.Debug(fmt.Sprintf(format, v...))
}

func (l zapLogger) Tracef(format string, v ...any) {
	l.logger.Debug(fmt.Sprintf(format, v...))
}
