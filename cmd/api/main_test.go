package main

import (
	"strings"
	"testing"
)

func TestConfiguration(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("DATABASE_URL", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("missing DATABASE_URL accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://ledger:ledger@localhost/ledger")
	config, err := loadConfig()
	if err != nil || config.address != "127.0.0.1:8080" {
		t.Fatalf("incorrect defaults: %+v %v", config, err)
	}
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	config, err = loadConfig()
	if err != nil || config.address != "127.0.0.1:9090" {
		t.Fatal("HTTP_ADDR override not used")
	}
	t.Setenv("DATABASE_URL", "postgres://user:secret@host:invalid/db")
	if _, err := loadConfig(); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("invalid configuration accepted or credential leaked")
	}
}
