package main

import "testing"

func TestHelpReturnsSuccess(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Fatalf("help returned a failure: %v", err)
	}
}

func TestRequiredConfigurationAndPositionalArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"unexpected"}, {"--config", "/definitely/missing/node.json"}} {
		if err := run(args); err == nil {
			t.Fatalf("invalid invocation accepted: %v", args)
		}
	}
}
