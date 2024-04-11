package main

import (
	"fmt"
	"testing"
)

func TestMakeGsePluginVersion(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		err      error
	}{
		{input: "1.2.3", expected: "1.0.102.3", err: nil},
		{input: "5.12.0", expected: "1.0.512.0", err: nil},
		{input: "5.0.99", expected: "1.0.500.99", err: nil},
		{input: "invalid", expected: "", err: fmt.Errorf("Malformed version: %s", "invalid")},
	}

	for _, c := range cases {
		got, err := makeGsePluginVersion(c.input)
		if got != c.expected || err != nil && err.Error() != c.err.Error() {
			t.Errorf("makeGsePluginVersion(%q) == %q, %v, want %q, %v", c.input, got, err, c.expected, c.err)
		}
	}
}
