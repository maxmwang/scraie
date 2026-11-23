package option

import "testing"

func TestOptionValue(t *testing.T) {
	value := 42
	option := Some(value)

	got, ok := option.Value()
	if !ok {
		t.Fatal("Value() reported no value")
	}
	if got != value {
		t.Fatalf("Value() = %d, want %d", got, value)
	}
}

func TestOptionNone(t *testing.T) {
	option := None[string]()

	got, ok := option.Value()
	if ok {
		t.Fatal("Value() reported a value")
	}
	if got != "" {
		t.Fatalf("Value() = %q, want zero value", got)
	}
	if got := option.ValueOrDefault("default"); got != "default" {
		t.Fatalf("ValueOrDefault() = %q, want default", got)
	}
}
