package main

import (
	"testing"
)

func TestIsLiteralRegexp(t *testing.T) {
	tests := []struct {
		pattern  string
		isRegexp bool
	}{
		{`foo`, true},
		{`.foo`, false},
		{`\foo`, false},
		{`(foo)`, false},
		{`/foo/`, true},
		{`%foo%`, true},
	}

	for _, test := range tests {
		value := isLiteralRegexp(test.pattern)
		expected := test.isRegexp
		if value != expected {
			t.Fatalf("Pattern isLiteralRegexp(%v) is %v but %v:", test.pattern, expected, value)
		}
	}
}
