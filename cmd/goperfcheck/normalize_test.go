package main

import (
	"reflect"
	"testing"
)

func TestNormalizeSpacedBoolFlags(t *testing.T) {
	bools := []string{"cache", "skip-vendor", "skip-tests", "verbose"}

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "space false is rewritten",
			in:   []string{"-cache", "false"},
			want: []string{"-cache=false"},
		},
		{
			name: "space true is rewritten",
			in:   []string{"-cache", "true"},
			want: []string{"-cache=true"},
		},
		{
			name: "space 0 is rewritten",
			in:   []string{"-cache", "0"},
			want: []string{"-cache=0"},
		},
		{
			name: "space 1 is rewritten",
			in:   []string{"-cache", "1"},
			want: []string{"-cache=1"},
		},
		{
			name: "double-dash variant is rewritten",
			in:   []string{"--cache", "false"},
			want: []string{"--cache=false"},
		},
		{
			name: "equals form passes through unchanged",
			in:   []string{"-cache=false"},
			want: []string{"-cache=false"},
		},
		{
			name: "bare flag (no value) passes through unchanged",
			in:   []string{"-cache"},
			want: []string{"-cache"},
		},
		{
			name: "non-bool flag space value is NOT merged",
			in:   []string{"-dir", "false"},
			want: []string{"-dir", "false"},
		},
		{
			name: "positional arg that happens to be false is not consumed",
			in:   []string{"-dir", "mydir", "false"},
			want: []string{"-dir", "mydir", "false"},
		},
		{
			name: "mixed args: bool flag with space value plus string flag",
			in:   []string{"-cache", "false", "-dir", "."},
			want: []string{"-cache=false", "-dir", "."},
		},
		{
			name: "mixed case bool value is normalised (TRUE → true)",
			in:   []string{"-cache", "TRUE"},
			want: []string{"-cache=TRUE"}, // value is passed through verbatim; strconv.ParseBool handles case
		},
		{
			name: "skip-vendor with space false",
			in:   []string{"-skip-vendor", "false"},
			want: []string{"-skip-vendor=false"},
		},
		{
			name: "non-boolean literal after flag is left as positional",
			in:   []string{"-cache", "maybe"},
			want: []string{"-cache", "maybe"},
		},
		{
			name: "empty args",
			in:   []string{},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSpacedBoolFlags(tt.in, bools)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalizeSpacedBoolFlags(%v) =\n  %v\nwant\n  %v", tt.in, got, tt.want)
			}
		})
	}
}
