package main

import (
	"strings"
	"testing"
)

// The version is a source constant so that a packaging pin claiming a
// different one is caught; this only guards its shape.
func TestVersionIsSet(t *testing.T) {
	if version == "" || strings.HasPrefix(version, "v") {
		t.Errorf("version = %q, want a bare semver like 0.1.0", version)
	}
}
