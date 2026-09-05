package logkeys

import "testing"

func TestCorrelationKeys(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "trace", got: TraceID, want: "trace_id"},
		{name: "build", got: BuildID, want: "build_id"},
		{name: "config", got: ConfigID, want: "config_id"},
		{name: "user", got: UserID, want: "user_id"},
	}
	for _, test := range tests {
		if test.got != test.want {
			t.Errorf("%s log key = %q, want %q", test.name, test.got, test.want)
		}
	}
}
