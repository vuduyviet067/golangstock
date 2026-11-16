package mapper_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Fixed instants shared by the mapper tests. createdAt has nanoseconds so the
// Auth (seconds) and User (nanoseconds) timestamp rules give different text;
// updatedAt ends in zeros to pin the RFC3339Nano trimming.
var (
	createdAt = time.Date(2026, 3, 4, 5, 6, 7, 891234567, time.UTC)
	updatedAt = time.Date(2026, 3, 5, 6, 7, 8, 120000000, time.UTC)
	ict       = time.FixedZone("ICT", 7*60*60)
	nst       = time.FixedZone("NST", -(5*60*60 + 30*60))
)

const passwordHash = "$2a$10$mapper-secret-hash-must-never-be-copied"

// assertJSON marshals v and compares it byte for byte with want.
func assertJSON(t *testing.T, v any, want string) {
	t.Helper()
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	if string(got) != want {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

// assertNoSecretFields fails if the type of v has a field that could carry a
// password or a hash, or if its JSON mentions one.
func assertNoSecretFields(t *testing.T, v any) {
	t.Helper()
	typ := reflect.TypeOf(v)
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name + " " + typ.Field(i).Tag.Get("json"))
		if strings.Contains(name, "password") || strings.Contains(name, "hash") {
			t.Errorf("%s has sensitive field %q", typ, typ.Field(i).Name)
		}
	}
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "password") || strings.Contains(lower, "hash") || strings.Contains(string(body), passwordHash) {
		t.Errorf("JSON exposes a password or hash: %s", body)
	}
}
