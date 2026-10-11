package ai

// This file verifies reopened counselor results retain strict structure and read-only safety.

import (
	"strings"
	"testing"
)

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

func TestDecodeStoredCounselorResponseRejectsInsecureCitationAndOversizedContent(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].Citations = []CounselorCitation{{Title: "Legacy HTTP", URL: "http://example.com/source"}}
	if _, err := DecodeStoredCounselorResponse(encodeCounselorTestResponse(t, response)); err == nil {
		t.Fatal("stored HTTP citation was accepted")
	}
	response = validCounselorTestResponse()
	response.Summary = strings.Repeat("x", maxSummaryCharacters+1)
	if _, err := DecodeStoredCounselorResponse(encodeCounselorTestResponse(t, response)); err == nil {
		t.Fatal("oversized stored response was accepted")
	}
}
