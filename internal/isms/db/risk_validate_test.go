package db

import (
	"errors"
	"testing"
)

// Risk.Validate must return *ValidationError so the API layer can answer 400
// instead of 500 (#296).
func TestRiskValidateReturnsValidationError(t *testing.T) {
	bad := 9
	r := &Risk{Title: "t", RiskType: "threat", Origin: "internal", Status: "open", CurrentLikelihood: &bad}
	err := r.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate() = %v (%T), want *ValidationError", err, err)
	}
	if got, want := err.Error(), "current_likelihood must be 0-5"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRiskValidateValidRisk(t *testing.T) {
	ok := 3
	r := &Risk{Title: "t", RiskType: "threat", Origin: "internal", Status: "open", CurrentLikelihood: &ok, CurrentImpact: &ok}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// #414: Validate() used to carry its own second hardcoded status map
// (draft/open/closed) alongside RiskStatuses — a status this package itself
// considers valid, like "accepted", would still have failed here even after
// RiskStatuses grew to include it. The API layer's validateEnum(RiskStatuses)
// always runs first in practice, so this exact branch is defense in depth
// rather than something the HTTP tests reach — covered directly here instead.
func TestRiskValidateAcceptsEveryRiskStatus(t *testing.T) {
	for _, status := range RiskStatuses {
		r := &Risk{Title: "t", RiskType: "threat", Origin: "internal", Status: status}
		if err := r.Validate(); err != nil {
			t.Errorf("Validate() with Status=%q: %v, want nil", status, err)
		}
	}
}

func TestRiskValidateRejectsUnknownStatus(t *testing.T) {
	r := &Risk{Title: "t", RiskType: "threat", Origin: "internal", Status: "treating"}
	err := r.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate() = %v (%T), want *ValidationError", err, err)
	}
}
