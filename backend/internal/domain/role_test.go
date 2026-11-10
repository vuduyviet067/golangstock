package domain

import "testing"

// TestRoleValues pins the role values. They are a contract with the database
// CHECK constraint, issued JWTs, and API clients, so changing them must fail.
func TestRoleValues(t *testing.T) {
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q, want %q", RoleUser, "user")
	}
	if RoleAdmin != "admin" {
		t.Errorf("RoleAdmin = %q, want %q", RoleAdmin, "admin")
	}
}

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{role: "user", want: true},
		{role: "admin", want: true},
		{role: "", want: false},
		{role: "Admin", want: false},
		{role: "ADMIN", want: false},
		{role: " admin", want: false},
		{role: "admin ", want: false},
		{role: "superadmin", want: false},
		{role: "guest", want: false},
	}
	for _, tt := range tests {
		if got := IsValidRole(tt.role); got != tt.want {
			t.Errorf("IsValidRole(%q) = %v, want %v", tt.role, got, tt.want)
		}
	}
}
