package osc7501

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestParse(t *testing.T) {
	seg32 := strings.Repeat("a", 32)
	id128 := strings.Join([]string{seg32, seg32, seg32, strings.Repeat("a", 29)}, "/")
	id129 := id128 + "b"
	title192 := strings.Repeat("a", 192)
	msg2048 := strings.Repeat("a", 2048)

	tests := []struct {
		name string
		body string
		want Report
		ok   bool
	}{
		{"minimal", "state=idle", Report{State: "idle", Progress: -1}, true},
		{"all states", "state=clear", Report{State: "clear", Progress: -1}, true},
		{"missing state", "id=a", Report{}, false},
		{"unknown state", "state=sleeping", Report{}, false},
		{"empty body", "", Report{}, false},
		{"query is not a report", "?", Report{}, false},
		{"whitespace stripped", " state = working : progress = 5 ", Report{State: "working", Progress: 5}, true},
		{"unknown key ignored", "state=done:foo=bar", Report{State: "done", Progress: -1}, true},
		{"repeated key last wins", "state=idle:state=done", Report{State: "done", Progress: -1}, true},
		{"malformed pair no equals", "state=done:junk:app=x", Report{State: "done", App: "x", Progress: -1}, true},
		{"malformed pair bad key", "state=done:Bad=1:app=x", Report{State: "done", App: "x", Progress: -1}, true},
		{"malformed pair bad value", "state=blocked:app=a b:kind=auth", Report{State: "blocked", Kind: "auth", Progress: -1}, true},
		{"kind ignored unless blocked", "state=working:kind=auth", Report{State: "working", Progress: -1}, true},
		{"bad pair does not override earlier", "state=done:state=bo!gus", Report{State: "done", Progress: -1}, true},
		{"id root absent", "state=done", Report{State: "done", Progress: -1}, true},
		{"id nested", "state=working:id=a/b.c/d-e", Report{State: "working", ID: "a/b.c/d-e", Progress: -1}, true},
		{"id segment 32", "state=done:id=" + seg32, Report{State: "done", ID: seg32, Progress: -1}, true},
		{"id segment 33", "state=done:id=" + seg32 + "a", Report{}, false},
		{"id 8 levels", "state=done:id=a/b/c/d/e/f/g/h", Report{State: "done", ID: "a/b/c/d/e/f/g/h", Progress: -1}, true},
		{"id 9 levels", "state=done:id=a/b/c/d/e/f/g/h/i", Report{}, false},
		{"id 128 bytes", "state=done:id=" + id128, Report{State: "done", ID: id128, Progress: -1}, true},
		{"id 129 bytes", "state=done:id=" + id129, Report{}, false},
		{"id empty segment", "state=done:id=a//b", Report{}, false},
		{"id empty value", "state=done:id=", Report{}, false},
		{"id invalid char", "state=done:id=a,b", Report{}, false},
		{"kind blocked", "state=blocked:kind=permission", Report{State: "blocked", Kind: "permission", Progress: -1}, true},
		{"kind question", "state=blocked:kind=question", Report{State: "blocked", Kind: "question", Progress: -1}, true},
		{"kind unknown absent", "state=blocked:kind=other", Report{State: "blocked", Progress: -1}, true},
		{"progress 0", "state=working:progress=0", Report{State: "working", Progress: 0}, true},
		{"progress 100", "state=working:progress=100", Report{State: "working", Progress: 100}, true},
		{"progress 101", "state=working:progress=101", Report{State: "working", Progress: -1}, true},
		{"progress negative", "state=working:progress=-1", Report{State: "working", Progress: -1}, true},
		{"progress nan", "state=working:progress=abc", Report{State: "working", Progress: -1}, true},
		{"progress empty", "state=working:progress=", Report{State: "working", Progress: -1}, true},
		{"progress blocked", "state=blocked:progress=40", Report{State: "blocked", Progress: 40}, true},
		{"progress ignored for done", "state=done:progress=40", Report{State: "done", Progress: -1}, true},
		{"progress ignored for error", "state=error:progress=40", Report{State: "error", Progress: -1}, true},
		{"app valid", "state=done:app=claude-code_1.2+x", Report{State: "done", App: "claude-code_1.2+x", Progress: -1}, true},
		{"app 33 bytes absent", "state=done:app=" + seg32 + "a", Report{State: "done", Progress: -1}, true},
		{"app invalid absent", "state=done:app=a,b", Report{State: "done", Progress: -1}, true},
		{"title padded", "state=done:title=" + b64("Hi!!"), Report{State: "done", Title: "Hi!!", Progress: -1}, true},
		{"title unpadded", "state=done:title=" + strings.TrimRight(b64("Hi"), "="), Report{State: "done", Title: "Hi", Progress: -1}, true},
		{"title padded short", "state=done:title=" + b64("Hi"), Report{State: "done", Title: "Hi", Progress: -1}, true},
		{"title 192 decoded", "state=done:title=" + b64(title192), Report{State: "done", Title: title192, Progress: -1}, true},
		{"title 193 decoded", "state=done:title=" + b64(title192+"a"), Report{}, false},
		{"title bad base64", "state=done:title=a===", Report{}, false},
		{"title bad base64 chars in set", "state=done:title=a", Report{}, false},
		{"title invalid utf8", "state=done:title=" + base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe}), Report{}, false},
		{"title utf8", "state=done:title=" + b64("héllo ✓"), Report{State: "done", Title: "héllo ✓", Progress: -1}, true},
		{"title newline", "state=done:title=" + b64("a\nb"), Report{}, false},
		{"title tab", "state=done:title=" + b64("a\tb"), Report{}, false},
		{"title DEL", "state=done:title=" + b64("a\x7fb"), Report{}, false},
		{"title ESC", "state=done:title=" + b64("a\x1bb"), Report{}, false},
		{"title C1", "state=done:title=" + b64("a\u0085b"), Report{}, false},
		{"title 0x20 ok", "state=done:title=" + b64(" a "), Report{State: "done", Title: " a ", Progress: -1}, true},
		{"msg 2048 decoded", "state=done:msg=" + b64(msg2048), Report{State: "done", Msg: msg2048, Progress: -1}, true},
		{"msg 2049 decoded", "state=done:msg=" + b64(msg2048+"a"), Report{}, false},
		{"msg 2050 decoded", "state=done:msg=" + b64(msg2048+"aa"), Report{}, false},
		{"msg control", "state=done:msg=" + b64("a\rb"), Report{}, false},
		{"msg decoded and later pair still read", "state=blocked:msg=" + b64("ok") + ":kind=auth", Report{State: "blocked", Msg: "ok", Kind: "auth", Progress: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Parse([]byte(tt.body))
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseEncodedLimits(t *testing.T) {
	// 256 encoded bytes is the title limit; 260 is the next valid length.
	_, ok := Parse([]byte("state=done:title=" + strings.Repeat("QUJD", 64)))
	require.True(t, ok)
	_, ok = Parse([]byte("state=done:title=" + strings.Repeat("QUJD", 64) + "Q"))
	require.False(t, ok)

	// 2732 encoded bytes is the msg limit.
	_, ok = Parse([]byte("state=done:msg=" + strings.Repeat("QUJD", 682) + "QUI="))
	require.True(t, ok)
	_, ok = Parse([]byte("state=done:msg=" + strings.Repeat("QUJD", 682) + "QUI=" + "Q"))
	require.False(t, ok)
}

func TestEncodeRoundTrip(t *testing.T) {
	reports := []Report{
		{State: "idle", Progress: -1},
		{State: "working", ID: "build/compile", Progress: 42, App: "make", Title: "Build", Msg: "compiling é"},
		{State: "blocked", Kind: "permission", Progress: -1, Msg: "Allow rm -rf?"},
		{State: "done", ID: "a", Progress: -1, Title: "x"},
		{State: "working", Progress: 0},
	}
	for _, r := range reports {
		enc := r.Encode()
		require.True(t, strings.HasPrefix(string(enc), "\x1b]7501;state="))
		require.True(t, strings.HasSuffix(string(enc), "\x1b\\"))
		body := enc[len("\x1b]7501;") : len(enc)-2]
		got, ok := Parse(body)
		require.True(t, ok)
		require.Equal(t, r, got)
	}
}

func TestEncodeCanonical(t *testing.T) {
	r := Report{State: "blocked", ID: "a/b", Kind: "auth", Progress: 7, App: "x", Title: "T", Msg: "M"}
	require.Equal(t,
		"\x1b]7501;state=blocked:id=a/b:kind=auth:progress=7:app=x:title=VA==:msg=TQ==\x1b\\",
		string(r.Encode()))
	require.Equal(t, "\x1b]7501;state=done\x1b\\", string(Report{State: "done", Progress: -1}.Encode()))
}
