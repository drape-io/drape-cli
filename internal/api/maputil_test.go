package api

import "testing"

func TestGetInt(t *testing.T) {
	m := map[string]any{"count": float64(42), "name": "foo"}

	if v := getInt(m, "count"); v == nil || *v != 42 {
		t.Errorf("getInt(count) = %v, want 42", v)
	}
	if v := getInt(m, "missing"); v != nil {
		t.Errorf("getInt(missing) = %v, want nil", v)
	}
	if v := getInt(m, "name"); v != nil {
		t.Errorf("getInt(name) = %v, want nil (wrong type)", v)
	}
}

func TestGetIntVal(t *testing.T) {
	m := map[string]any{"count": float64(42)}

	if v := getIntVal(m, "count"); v != 42 {
		t.Errorf("getIntVal(count) = %d, want 42", v)
	}
	if v := getIntVal(m, "missing"); v != 0 {
		t.Errorf("getIntVal(missing) = %d, want 0", v)
	}
}

func TestGetString(t *testing.T) {
	m := map[string]any{"name": "foo", "count": float64(42)}

	if v := getString(m, "name"); v == nil || *v != "foo" {
		t.Errorf("getString(name) = %v, want foo", v)
	}
	if v := getString(m, "missing"); v != nil {
		t.Errorf("getString(missing) = %v, want nil", v)
	}
	if v := getString(m, "count"); v != nil {
		t.Errorf("getString(count) = %v, want nil (wrong type)", v)
	}
}

func TestGetStringVal(t *testing.T) {
	m := map[string]any{"name": "foo"}

	if v := getStringVal(m, "name"); v != "foo" {
		t.Errorf("getStringVal(name) = %q, want foo", v)
	}
	if v := getStringVal(m, "missing"); v != "" {
		t.Errorf("getStringVal(missing) = %q, want empty", v)
	}
}

func TestGetBool(t *testing.T) {
	m := map[string]any{"passed": true, "name": "foo"}

	if v := getBool(m, "passed"); !v {
		t.Errorf("getBool(passed) = %v, want true", v)
	}
	if v := getBool(m, "missing"); v {
		t.Errorf("getBool(missing) = %v, want false", v)
	}
	if v := getBool(m, "name"); v {
		t.Errorf("getBool(name) = %v, want false (wrong type)", v)
	}
}

func TestGetStringSlice(t *testing.T) {
	m := map[string]any{
		"tags":    []any{"a", "b", "c"},
		"mixed":   []any{"a", float64(1), "b"},
		"empty":   []any{},
		"notlist": "foo",
	}

	if v := getStringSlice(m, "tags"); len(v) != 3 || v[0] != "a" || v[2] != "c" {
		t.Errorf("getStringSlice(tags) = %v, want [a b c]", v)
	}
	if v := getStringSlice(m, "mixed"); len(v) != 2 || v[0] != "a" || v[1] != "b" {
		t.Errorf("getStringSlice(mixed) = %v, want [a b] (skipping non-strings)", v)
	}
	if v := getStringSlice(m, "empty"); len(v) != 0 {
		t.Errorf("getStringSlice(empty) = %v, want empty", v)
	}
	if v := getStringSlice(m, "missing"); v != nil {
		t.Errorf("getStringSlice(missing) = %v, want nil", v)
	}
	if v := getStringSlice(m, "notlist"); v != nil {
		t.Errorf("getStringSlice(notlist) = %v, want nil (wrong type)", v)
	}
}
