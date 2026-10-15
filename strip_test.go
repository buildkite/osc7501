package osc7501

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func stripAll(chunks ...string) string {
	var st Stripper
	var out []byte
	for _, c := range chunks {
		out = append(out, st.Strip([]byte(c))...)
	}
	out = append(out, st.Flush()...)
	return string(out)
}

func TestStripper(t *testing.T) {
	over := "\x1b]7501;state=working:msg=" + strings.Repeat("a", 5000) + "\x07"
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"plain", []string{"hello\n"}, "hello\n"},
		{"bel", []string{"a\x1b]7501;state=working\x07b"}, "ab"},
		{"st", []string{"a\x1b]7501;state=done:id=x\x1b\\b"}, "ab"},
		{"query", []string{"a\x1b]7501;?\x07b"}, "ab"},
		{"non-7501 osc kept", []string{"a\x1b]0;title\x07b"}, "a\x1b]0;title\x07b"},
		{"non-7501 osc st kept", []string{"a\x1b]0;title\x1b\\b"}, "a\x1b]0;title\x1b\\b"},
		{"CAN abort kept", []string{"\x1b]7501;state=working\x18rest"}, "\x1b]7501;state=working\x18rest"},
		{"esc esc backslash kept", []string{"\x1b]7501;state=working\x1b\x1b\\x"}, "\x1b]7501;state=working\x1b\x1b\\x"},
		{"esc bracket restarts", []string{"\x1b]7501;state=a\x1b]7501;state=done\x07z"}, "\x1b]7501;state=az"},
		{"over cap kept", []string{over + "x"}, over + "x"},
		{"partial then flush", []string{"a\x1b]7501;state=wor"}, "a\x1b]7501;state=wor"},
		{"lone esc flush", []string{"a\x1b"}, "a\x1b"},
		{"esc other", []string{"a\x1b[31mb"}, "a\x1b[31mb"},
		{"split", []string{"a\x1b", "]75", "01;state=do", "ne\x1b", "\\b"}, "ab"},
		{"two sequences", []string{"\x1b]7501;state=a\x07mid\x1b]7501;?\x07"}, "mid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, stripAll(tt.in...))
		})
	}
}

func TestStripperHoldsUntilResolved(t *testing.T) {
	var st Stripper
	require.Empty(t, st.Strip([]byte("\x1b]7501;sta")))
	require.Equal(t, "\x1b]7501;sta", string(st.Flush()))
	require.Empty(t, st.Flush())
	require.Equal(t, "x", string(st.Strip([]byte("x"))))
}

func FuzzStripper(f *testing.F) {
	f.Add([]byte("a\x1b]7501;state=working\x07b"), uint(5))
	f.Add([]byte("\x1b]7501;?\x1b\\\x1b]0;t\x07"), uint(3))
	f.Add([]byte("\x1b]7501;x\x1b\x1b\\\x18\x1b]\x1b]7501;state=done\x07"), uint(9))
	f.Fuzz(func(t *testing.T, data []byte, split uint) {
		var one Stripper
		want := append(append([]byte{}, one.Strip(data)...), one.Flush()...)

		cut := int(split % uint(len(data)+1))
		var two Stripper
		got := append([]byte{}, two.Strip(data[:cut])...)
		got = append(got, two.Strip(data[cut:])...)
		got = append(got, two.Flush()...)
		require.Equal(t, want, got)

		if !bytes.Contains(data, []byte{0x1b}) {
			require.Equal(t, data, want)
		}
		require.LessOrEqual(t, len(want), len(data))
	})
}
