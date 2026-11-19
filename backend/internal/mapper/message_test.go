package mapper_test

import (
	"encoding/json"
	"testing"

	"gostock/internal/dto"
	"gostock/internal/mapper"
)

func TestToMessageResponse(t *testing.T) {
	got := mapper.ToMessageResponse("done")

	if got != (dto.MessageResponse{Message: "done"}) {
		t.Fatalf("ToMessageResponse = %+v, want {Message:done}", got)
	}
	assertJSON(t, got, `{"message":"done"}`)
	assertJSON(t, mapper.ToMessageResponse(""), `{"message":""}`)
}

func TestToLogoutResponse(t *testing.T) {
	if mapper.LogoutMessage != "logged out" {
		t.Fatalf("LogoutMessage = %q, want %q", mapper.LogoutMessage, "logged out")
	}
	assertJSON(t, mapper.ToLogoutResponse(), `{"message":"logged out"}`)

	// The DTO serializes exactly like the map the logout handler sends today.
	legacy, err := json.Marshal(map[string]string{"message": "logged out"})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, mapper.ToLogoutResponse(), string(legacy))
}
