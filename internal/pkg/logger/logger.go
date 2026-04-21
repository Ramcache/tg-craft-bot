package logger

import (
	"fmt"

	"go.uber.org/zap"
)

func New(level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if err := cfg.Level.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	return cfg.Build()
}
