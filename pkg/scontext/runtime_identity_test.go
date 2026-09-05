package scontext

import (
	"context"
	"testing"
)

func TestRuntimeIdentityHelpers(t *testing.T) {
	ctx := context.Background()
	ctx = WithBuildID[string, testUser](ctx, "build-1")
	ctx = WithConfigID[string, testUser](ctx, "config-1")

	if buildID, ok := GetBuildID[string, testUser](ctx); !ok || buildID != "build-1" {
		t.Fatalf("GetBuildID = (%q, %v), want (build-1, true)", buildID, ok)
	}
	if configID, ok := GetConfigID[string, testUser](ctx); !ok || configID != "config-1" {
		t.Fatalf("GetConfigID = (%q, %v), want (config-1, true)", configID, ok)
	}
}

func TestRuntimeIdentityHelpersDistinguishSetEmptyValues(t *testing.T) {
	ctx := WithBuildID[string, testUser](context.Background(), "")
	ctx = WithConfigID[string, testUser](ctx, "")

	if buildID, ok := GetBuildID[string, testUser](ctx); !ok || buildID != "" {
		t.Fatalf("GetBuildID = (%q, %v), want (empty, true)", buildID, ok)
	}
	if configID, ok := GetConfigID[string, testUser](ctx); !ok || configID != "" {
		t.Fatalf("GetConfigID = (%q, %v), want (empty, true)", configID, ok)
	}
}
