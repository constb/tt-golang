package database

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDatabaseConnection(t *testing.T) {
	url := os.Getenv("DB_URL")
	if url == "" {
		t.Skipf("db tests require database")
		return
	}

	for {
		dir, _ := os.Getwd()
		if len(dir) <= 1 {
			t.Skipf("project root folder")
			return
		}
		if strings.HasSuffix(dir, "tt-golang") {
			break
		}
		_ = os.Chdir("..")
	}

	db, err := NewDatabaseConnection()
	if err != nil {
		t.Errorf("connection error %v", err)
	}
	assert.NotEmpty(t, db, "database not nil")
}
