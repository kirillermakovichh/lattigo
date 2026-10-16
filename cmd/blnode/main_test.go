package main

import "testing"

func TestHelpReturnsSuccess(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Fatalf("help returned a failure: %v", err)
	}
}
