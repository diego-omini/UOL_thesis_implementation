package ai

// This file verifies reopened counselor results retain strict structure and read-only safety.

import "testing"

func TestDecodeStoredCounselorResponseRejectsUnsafeOrUnknownContent(t *testing.T) {
	unsafe := `{"schema_version":1,"summary":"You should buy TEST.","observations":[{"category":"general_information","statement":"General","supporting_fact_ids":[],"reported_values":[],"severity":"informational","confidence":"low","limitations":[],"explanation":"General context.","citations":[]}],"limitations":[{"statement":"Limited.","fact_ids":[]}]}`
	if _, err := DecodeStoredCounselorResponse(unsafe); err == nil {
		t.Fatal("unsafe saved guidance was accepted")
	}
	unknown := `{"schema_version":1,"summary":"Summary","unknown":true,"observations":[],"limitations":[]}`
	if _, err := DecodeStoredCounselorResponse(unknown); err == nil {
		t.Fatal("unknown saved response field was accepted")
	}
}
