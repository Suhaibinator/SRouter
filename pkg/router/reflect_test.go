package router

import (
	"reflect"
	"testing"
)

type reflectNested struct {
	ID string `json:"id"`
}

type reflectRequest struct {
	Name   string          `json:"name"`
	Age    int             `json:"age,omitempty"`
	Nested *reflectNested  `json:"nested"`
	Items  []reflectNested `json:"items,omitempty"`
	skipMe string
}

func TestBuildTypeSchema(t *testing.T) {
	schema := buildTypeSchema(reflect.TypeOf(reflectRequest{}))
	if schema == nil {
		t.Fatal("expected schema")
	}
	if schema.Kind != "struct" {
		t.Fatalf("expected struct kind, got %q", schema.Kind)
	}
	if len(schema.Fields) != 4 {
		t.Fatalf("expected 4 exported fields, got %d", len(schema.Fields))
	}
	if schema.Fields[0].JSONName != "name" || !schema.Fields[0].Required {
		t.Fatalf("unexpected first field metadata: %+v", schema.Fields[0])
	}
	if schema.Fields[1].Required {
		t.Fatalf("omitempty field should not be required: %+v", schema.Fields[1])
	}
	if schema.Fields[2].Schema == nil || schema.Fields[2].Schema.TypeName != "reflectNested" {
		t.Fatalf("expected nested struct schema, got: %+v", schema.Fields[2].Schema)
	}
	if schema.Fields[3].Schema == nil || schema.Fields[3].Schema.Kind != "struct" {
		t.Fatalf("expected slice element struct schema, got: %+v", schema.Fields[3].Schema)
	}
}
