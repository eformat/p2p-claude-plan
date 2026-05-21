package protocol

import "testing"

func TestIsValidPlanID(t *testing.T) {
	valid := []string{
		"my-plan",
		"plan_v2",
		"simple",
		"plan.with.dots",
		"CamelCase",
		"abc123",
		"a",
	}
	for _, id := range valid {
		if !isValidPlanID(id) {
			t.Errorf("expected valid: %q", id)
		}
	}

	invalid := []string{
		"",
		"../../../etc/passwd",
		"plan/subdir",
		"plan\\backslash",
		"plan with spaces",
		"plan\x00null",
		"plan;injection",
		"plan&cmd",
		"plan|pipe",
		"plan$(cmd)",
		"plan`backtick`",
		"{\"json\":true}",
		"plan\nwith\nnewlines",
		"plan\ttab",
	}
	for _, id := range invalid {
		if isValidPlanID(id) {
			t.Errorf("expected invalid: %q", id)
		}
	}
}

func TestIsValidPlanIDMaxLength(t *testing.T) {
	long := make([]byte, 256)
	for i := range long {
		long[i] = 'a'
	}
	if isValidPlanID(string(long)) {
		t.Error("256-char ID should be invalid (max 255)")
	}

	ok := make([]byte, 255)
	for i := range ok {
		ok[i] = 'a'
	}
	if !isValidPlanID(string(ok)) {
		t.Error("255-char ID should be valid")
	}
}
