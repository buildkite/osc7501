package osc7501

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestReporterHelpers(t *testing.T) {
	tests := []struct {
		name string
		call func(r *Reporter) error
		want string
		rep  Report
	}{
		{"working", func(r *Reporter) error { return r.Working("build", 25, "compiling") },
			"\x1b]7501;state=working:id=build:progress=25:app=demo:msg=" + b64("compiling") + "\x1b\\",
			Report{State: "working", ID: "build", Progress: 25, App: "demo", Msg: "compiling"}},
		{"blocked", func(r *Reporter) error { return r.Blocked("deploy", KindPermission, "ok?") },
			"\x1b]7501;state=blocked:id=deploy:kind=permission:app=demo:msg=" + b64("ok?") + "\x1b\\",
			Report{State: "blocked", ID: "deploy", Kind: "permission", Progress: -1, App: "demo", Msg: "ok?"}},
		{"done", func(r *Reporter) error { return r.Done("build", "") },
			"\x1b]7501;state=done:id=build:app=demo\x1b\\",
			Report{State: "done", ID: "build", Progress: -1, App: "demo"}},
		{"failed", func(r *Reporter) error { return r.Failed("build", "boom") },
			"\x1b]7501;state=error:id=build:app=demo:msg=" + b64("boom") + "\x1b\\",
			Report{State: "error", ID: "build", Progress: -1, App: "demo", Msg: "boom"}},
		{"idle", func(r *Reporter) error { return r.Idle("build") },
			"\x1b]7501;state=idle:id=build:app=demo\x1b\\",
			Report{State: "idle", ID: "build", Progress: -1, App: "demo"}},
		{"clear", func(r *Reporter) error { return r.Clear("") },
			"\x1b]7501;state=clear:app=demo\x1b\\",
			Report{State: "clear", Progress: -1, App: "demo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, tt.call(NewReporter(&buf, "demo")))
			require.Equal(t, tt.want, buf.String())

			body := strings.TrimSuffix(strings.TrimPrefix(buf.String(), "\x1b]7501;"), "\x1b\\")
			got, ok := Parse([]byte(body))
			require.True(t, ok)
			require.Equal(t, tt.rep, got)
		})
	}
}

func TestEncodeProgressAndKind(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, "demo")

	require.NoError(t, r.Working("b", -1, ""))
	require.Equal(t, "\x1b]7501;state=working:id=b:app=demo\x1b\\", buf.String())

	buf.Reset()
	require.NoError(t, r.Working("b", 150, ""))
	require.Equal(t, "\x1b]7501;state=working:id=b:progress=100:app=demo\x1b\\", buf.String())

	buf.Reset()
	require.NoError(t, r.Report(Report{State: "done", Progress: 50, Kind: "auth"}))
	require.Equal(t, "\x1b]7501;state=done:app=demo\x1b\\", buf.String())
}

func TestReporterInvalid(t *testing.T) {
	tests := map[string]Report{
		"state": {State: "nope"},
		"id":    {State: "done", ID: "a//b"},
		"app":   {State: "done", App: "bad app"},
		"kind":  {State: "blocked", Kind: "nope"},
	}
	for name, rep := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			err := NewReporter(&buf, "").Report(rep)
			require.True(t, errors.Is(err, ErrInvalidReport))
			require.Empty(t, buf.Bytes())
		})
	}
}

func TestEncodeSanitises(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, "demo")
	require.NoError(t, r.Done("b", "a\nb"))
	require.Contains(t, buf.String(), "msg="+b64("a b"))

	buf.Reset()
	require.NoError(t, r.Report(Report{State: "working", Title: strings.Repeat("é", 200), Progress: -1}))
	body := strings.TrimSuffix(strings.TrimPrefix(buf.String(), "\x1b]7501;"), "\x1b\\")
	got, ok := Parse([]byte(body))
	require.True(t, ok)
	require.LessOrEqual(t, len(got.Title), 192)
	require.True(t, utf8.ValidString(got.Title))
	require.Equal(t, strings.Repeat("é", 96), got.Title)
}

func TestCleanText(t *testing.T) {
	require.Equal(t, "a b c", cleanText("a\nb\x7fc", 100))
	require.Equal(t, "a b", cleanText("a\u0085b", 100))
	require.Equal(t, strings.Repeat("é", 5), cleanText(strings.Repeat("é", 100), 11))
	require.Equal(t, "abc", cleanText("abcdef", 3))
}

type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

func TestReporterWriteError(t *testing.T) {
	want := errors.New("boom")
	require.ErrorIs(t, NewReporter(errWriter{want}, "").Idle("x"), want)
}
