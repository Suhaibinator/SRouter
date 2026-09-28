package scontext

import "testing"

func TestParamsLookup(t *testing.T) {
	ps := Params{{Key: "id", Value: "42"}, {Key: "path", Value: ""}, {Key: "id", Value: "shadowed"}}

	tests := []struct {
		name      string
		wantValue string
		wantOK    bool
	}{
		{name: "id", wantValue: "42", wantOK: true},
		{name: "path", wantValue: "", wantOK: true},
		{name: "missing", wantValue: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := ps.Get(tt.name)
			if value != tt.wantValue || ok != tt.wantOK {
				t.Fatalf("Get(%q) = %q, %v; want %q, %v", tt.name, value, ok, tt.wantValue, tt.wantOK)
			}
			if got := ps.ByName(tt.name); got != tt.wantValue {
				t.Fatalf("ByName(%q) = %q; want %q", tt.name, got, tt.wantValue)
			}
		})
	}

	var empty Params
	if value, ok := empty.Get("id"); value != "" || ok {
		t.Fatalf("nil Params Get = %q, %v; want empty, false", value, ok)
	}
}
