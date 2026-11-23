package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	initLogger("error")
	os.Exit(m.Run())
}
