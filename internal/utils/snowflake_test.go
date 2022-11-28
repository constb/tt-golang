package utils

import (
	"testing"
)

func TestGenerateID(t *testing.T) {
	t.Parallel()

	if node == nil {
		t.Error("snowflake node is not initialized")
	}

	got := GenerateID()
	if got == 0 {
		t.Errorf("GenerateID() = %v, want actual new id", got)
	}
}
