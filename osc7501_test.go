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
		err  error
	}{
		{"minimal", "state=idle", Report{State: "idle", Progress: -1}, nil},
		{"all states", "state=clear", Report{State: "clear", Progress: -1}, nil},
		{"missing state", "id=a", Report{}, ErrNoState},
		{"unknown state", "state=sleeping", Report{}, ErrUnknownState},
		{"empty body", "", Report{}, ErrNoState},
		{"query is not a report", "?", Report{}, ErrNoState},
		{"whitespace stripped", " state = working : progress = 5 ", Report{State: "working", Progress: 5}, nil},
		{"unknown key ignored", "state=done:foo=bar", Report{State: "done", Progress: -1}, nil},
		{"repeated key last wins", "state=idle:state=done", Report{State: "done", Progress: -1}, nil},
		{"malformed pair no equals", "state=done:junk:app=x", Report{State: "done", App: "x", Progress: -1}, nil},
		{"malformed pair bad key", "state=done:Bad=1:app=x", Report{State: "done", App: "x", Progress: -1}, nil},
		{"malformed pair bad value", "state=blocked:app=a b:kind=auth", Report{State: "blocked", Kind: "auth", Progress: -1}, nil},
		{"kind ignored unless blocked", "state=working:kind=auth", Report{State: "working", Progress: -1}, nil},
		{"bad pair does not override earlier", "state=done:state=bo!gus", Report{State: "done", Progress: -1}, nil},
		{"key 16 bytes", "state=done:" + strings.Repeat("k", 16) + "=1", Report{State: "done", Progress: -1}, nil},
		{"key 17 bytes", "state=done:" + strings.Repeat("k", 17) + "=1", Report{}, ErrLimit},
		{"key 17 bytes malformed value", "state=done:" + strings.Repeat("k", 17) + "=a b", Report{}, ErrLimit},
		{"oversized id before valid", "state=done:id=" + id129 + ":id=a", Report{}, ErrInvalidID},
		{"oversized app before valid", "state=done:app=" + seg32 + "a:app=x", Report{}, ErrLimit},
		{"oversized title before valid", "state=done:title=" + b64(title192+"a") + ":title=QQ==", Report{}, ErrLimit},
		{"oversized msg before valid", "state=done:msg=" + b64(msg2048+"a") + ":msg=QQ==", Report{}, ErrLimit},
		{"bad base64 before valid", "state=done:msg=a:msg=QQ==", Report{}, ErrInvalidText},
		{"valid duplicate last wins", "state=done:msg=QQ==:msg=Qg==", Report{State: "done", Msg: "B", Progress: -1}, nil},
		{"id root absent", "state=done", Report{State: "done", Progress: -1}, nil},
		{"id nested", "state=working:id=a/b.c/d-e", Report{State: "working", ID: "a/b.c/d-e", Progress: -1}, nil},
		{"id segment 32", "state=done:id=" + seg32, Report{State: "done", ID: seg32, Progress: -1}, nil},
		{"id segment 33", "state=done:id=" + seg32 + "a", Report{}, ErrInvalidID},
		{"id 8 levels", "state=done:id=a/b/c/d/e/f/g/h", Report{State: "done", ID: "a/b/c/d/e/f/g/h", Progress: -1}, nil},
		{"id 9 levels", "state=done:id=a/b/c/d/e/f/g/h/i", Report{}, ErrInvalidID},
		{"id 128 bytes", "state=done:id=" + id128, Report{State: "done", ID: id128, Progress: -1}, nil},
		{"id 129 bytes", "state=done:id=" + id129, Report{}, ErrInvalidID},
		{"id empty segment", "state=done:id=a//b", Report{}, ErrInvalidID},
		{"id empty value", "state=done:id=", Report{}, ErrInvalidID},
		{"id invalid char", "state=done:id=a,b", Report{}, ErrInvalidID},
		{"kind blocked", "state=blocked:kind=permission", Report{State: "blocked", Kind: "permission", Progress: -1}, nil},
		{"kind question", "state=blocked:kind=question", Report{State: "blocked", Kind: "question", Progress: -1}, nil},
		{"kind unknown absent", "state=blocked:kind=other", Report{State: "blocked", Progress: -1}, nil},
		{"progress 0", "state=working:progress=0", Report{State: "working", Progress: 0}, nil},
		{"progress 100", "state=working:progress=100", Report{State: "working", Progress: 100}, nil},
		{"progress 101", "state=working:progress=101", Report{State: "working", Progress: -1}, nil},
		{"progress negative", "state=working:progress=-1", Report{State: "working", Progress: -1}, nil},
		{"progress nan", "state=working:progress=abc", Report{State: "working", Progress: -1}, nil},
		{"progress empty", "state=working:progress=", Report{State: "working", Progress: -1}, nil},
		{"progress blocked", "state=blocked:progress=40", Report{State: "blocked", Progress: 40}, nil},
		{"progress ignored for done", "state=done:progress=40", Report{State: "done", Progress: -1}, nil},
		{"progress ignored for error", "state=error:progress=40", Report{State: "error", Progress: -1}, nil},
		{"app valid", "state=done:app=claude-code_1.2+x", Report{State: "done", App: "claude-code_1.2+x", Progress: -1}, nil},
		{"app 33 bytes", "state=done:app=" + seg32 + "a", Report{}, ErrLimit},
		{"app invalid absent", "state=done:app=a,b", Report{State: "done", Progress: -1}, nil},
		{"title padded", "state=done:title=" + b64("Hi!!"), Report{State: "done", Title: "Hi!!", Progress: -1}, nil},
		{"title unpadded", "state=done:title=" + strings.TrimRight(b64("Hi"), "="), Report{State: "done", Title: "Hi", Progress: -1}, nil},
		{"title padded short", "state=done:title=" + b64("Hi"), Report{State: "done", Title: "Hi", Progress: -1}, nil},
		{"title 192 decoded", "state=done:title=" + b64(title192), Report{State: "done", Title: title192, Progress: -1}, nil},
		{"title 193 decoded", "state=done:title=" + b64(title192+"a"), Report{}, ErrLimit},
		{"title bad base64", "state=done:title=a===", Report{}, ErrInvalidText},
		{"title bad base64 chars in set", "state=done:title=a", Report{}, ErrInvalidText},
		{"title invalid utf8", "state=done:title=" + base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe}), Report{}, ErrInvalidText},
		{"title utf8", "state=done:title=" + b64("héllo ✓"), Report{State: "done", Title: "héllo ✓", Progress: -1}, nil},
		{"title newline", "state=done:title=" + b64("a\nb"), Report{}, ErrInvalidText},
		{"title tab", "state=done:title=" + b64("a\tb"), Report{}, ErrInvalidText},
		{"title DEL", "state=done:title=" + b64("a\x7fb"), Report{}, ErrInvalidText},
		{"title ESC", "state=done:title=" + b64("a\x1bb"), Report{}, ErrInvalidText},
		{"title C1", "state=done:title=" + b64("a\u0085b"), Report{}, ErrInvalidText},
		{"title 0x20 ok", "state=done:title=" + b64(" a "), Report{State: "done", Title: " a ", Progress: -1}, nil},
		{"msg 2048 decoded", "state=done:msg=" + b64(msg2048), Report{State: "done", Msg: msg2048, Progress: -1}, nil},
		{"msg 2049 decoded", "state=done:msg=" + b64(msg2048+"a"), Report{}, ErrLimit},
		{"msg 2050 decoded", "state=done:msg=" + b64(msg2048+"aa"), Report{}, ErrLimit},
		{"msg control", "state=done:msg=" + b64("a\rb"), Report{}, ErrInvalidText},
		{"msg decoded and later pair still read", "state=blocked:msg=" + b64("ok") + ":kind=auth", Report{State: "blocked", Msg: "ok", Kind: "auth", Progress: -1}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.body))
			if tt.err == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseEncodedLimits(t *testing.T) {
	// 256 encoded bytes is the title limit; 260 is the next valid length.
	_, err := Parse([]byte("state=done:title=" + strings.Repeat("QUJD", 64)))
	require.NoError(t, err)
	_, err = Parse([]byte("state=done:title=" + strings.Repeat("QUJD", 64) + "Q"))
	require.ErrorIs(t, err, ErrLimit)

	// 2732 encoded bytes is the msg limit.
	_, err = Parse([]byte("state=done:msg=" + strings.Repeat("QUJD", 682) + "QUI="))
	require.NoError(t, err)
	_, err = Parse([]byte("state=done:msg=" + strings.Repeat("QUJD", 682) + "QUI=" + "Q"))
	require.ErrorIs(t, err, ErrLimit)
}

func TestParseBodyLimit(t *testing.T) {
	// ESC ] "7501;" body BEL must fit in 4096 bytes.
	body := "state=done:x=" + strings.Repeat("a", maxBody-len("state=done:x="))
	_, err := Parse([]byte(body))
	require.NoError(t, err)
	_, err = Parse([]byte(body + "a"))
	require.ErrorIs(t, err, ErrLimit)
}

func TestParseErrorMessages(t *testing.T) {
	tests := []struct{ body, want string }{
		{"id=a", "osc7501: missing state"},
		{"state=sleeping", `osc7501: unknown state: "sleeping"`},
		{"state=done:" + strings.Repeat("k", 17) + "=1", "osc7501: limit exceeded: key is 17 bytes, max 16"},
		{"state=done:app=" + strings.Repeat("a", 33), "osc7501: limit exceeded: app is 33 bytes, max 32"},
		{"state=done:id=a//b", `osc7501: invalid id: "a//b"`},
		{"state=done:msg=a", "osc7501: invalid text: msg is not valid base64"},
		{"state=done:msg=" + b64(strings.Repeat("a", 2049)), "osc7501: limit exceeded: msg is 2049 bytes decoded, max 2048"},
		{"state=done:title=" + b64("a\nb"), "osc7501: invalid text: title contains control character U+000A"},
	}
	for _, tt := range tests {
		_, err := Parse([]byte(tt.body))
		require.EqualError(t, err, tt.want)
	}
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
		got, err := Parse(body)
		require.NoError(t, err)
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
