package server

import (
	"reflect"
	"testing"
)

func TestIsSensitiveKey(t *testing.T) {
	cases := map[string]bool{
		"password":       true,
		"Token":          true,
		"smtp_pass":      true,
		"encryption_key": true,
		"webhook_url":    true,
		"dsn":            true,
		"host":           false,
		"port":           false,
		"enabled":        false,
		"prefix":         false,
	}
	for key, want := range cases {
		if got := isSensitiveKey(key); got != want {
			t.Errorf("isSensitiveKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestRedactMap(t *testing.T) {
	in := map[string]any{
		"server": map[string]any{
			"token": "tok_secret",
			"host":  "example.com",
			"empty_password": "",
		},
		"list": []any{
			map[string]any{"secret": "hunter2", "name": "keep"},
		},
	}
	got := redactMap(in).(map[string]any)

	srv := got["server"].(map[string]any)
	if srv["token"] != redactValue {
		t.Errorf("token not redacted: %v", srv["token"])
	}
	if srv["host"] != "example.com" {
		t.Errorf("host wrongly altered: %v", srv["host"])
	}
	if srv["empty_password"] != "" {
		t.Errorf("empty secret should stay empty: %v", srv["empty_password"])
	}
	lst := got["list"].([]any)
	item := lst[0].(map[string]any)
	if item["secret"] != redactValue {
		t.Errorf("nested list secret not redacted: %v", item["secret"])
	}
	if item["name"] != "keep" {
		t.Errorf("non-secret list field altered: %v", item["name"])
	}
}

func TestRedactMap_NonMapPassthrough(t *testing.T) {
	in := "plain"
	if got := redactMap(in); !reflect.DeepEqual(got, in) {
		t.Errorf("scalar passthrough failed: %v", got)
	}
}
