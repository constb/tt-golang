package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLogger(t *testing.T) {
	type args struct {
		appName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"not nil", args{"testing"}, "testing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdLogger := NewLogger(tt.args.appName)
			assert.NotEmptyf(t, createdLogger, "logger create")
			assert.Equalf(t, logger, createdLogger, "global logger set")
		})
	}
}
