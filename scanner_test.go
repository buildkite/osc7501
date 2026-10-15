package osc7501

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type capture struct {
	out     bytes.Buffer
	reports []Report
	queries int
}

func (c *capture) scanner() *Scanner {
	return &Scanner{
		W:        &c.out,
		OnReport: func(r Report) { c.reports = append(c.reports, r) },
		OnQuery:  func() { c.queries++ },
	}
}

func feed(t *testing.T, in []byte, cuts ...int) *capture {
	t.Helper()
	c := &capture{}
	s := c.scanner()
	prev := 0
	for _, cut := range append(cuts, len(in)) {
		n, err := s.Write(in[prev:cut])
		require.NoError(t, err)
		require.Equal(t, cut-prev, n)
		prev = cut
	}
	return c
}

var wantReport = Report{State: "working", ID: "build", Progress: 50, App: "make", Msg: "hello"}

func TestScannerSplits(t *testing.T) {
	for _, term := range []string{"\x1b\\", "\x07"} {
		seq := string(wantReport.Encode())
		seq = strings.TrimSuffix(seq, "\x1b\\") + term
		in := []byte("before\r\n" + seq + "after\x1b[0m")

		c := feed(t, in)
		require.Equal(t, in, c.out.Bytes())
		require.Equal(t, []Report{wantReport}, c.reports)

		// 1-byte writes.
		cuts := make([]int, 0, len(in))
		for i := 1; i < len(in); i++ {
			cuts = append(cuts, i)
		}
		c = feed(t, in, cuts...)
		require.Equal(t, in, c.out.Bytes())
		require.Equal(t, []Report{wantReport}, c.reports)

		// Every 2-way split.
		for i := 0; i <= len(in); i++ {
			c = feed(t, in, i)
			require.Equal(t, in, c.out.Bytes(), "split %d", i)
			require.Equal(t, []Report{wantReport}, c.reports, "split %d", i)
		}
	}
}

func TestScannerNoCallback(t *testing.T) {
	long := "\x1b]7501;state=done:msg=" + strings.Repeat("A", 4100) + "\x1b\\"
	tests := map[string]string{
		"osc 0 title":         "\x1b]0;window title\x07",
		"osc 75010":           "\x1b]75010;state=done\x07",
		"osc 750":             "\x1b]750;state=done\x07",
		"overflow":            long,
		"CAN abort":           "\x1b]7501;state=done\x18\x07",
		"SUB abort":           "\x1b]7501;state=done\x1a\x07",
		"stray ESC abort":     "\x1b]7501;state=done\x1b[0m\x07",
		"ESC ESC backslash":   "\x1b]7501;state=done\x1b\x1b\\",
		"invalid report":      "\x1b]7501;state=bogus\x07",
		"unterminated":        "\x1b]7501;state=done",
		"plain text":          "state=done\x07",
		"csi then terminator": "\x1b[7501;state=done\x1b\\",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			c := feed(t, []byte("x"+in+"y"))
			require.Equal(t, "x"+in+"y", c.out.String())
			require.Empty(t, c.reports)
			require.Zero(t, c.queries)
		})
	}
}

func TestScannerSizeBoundary(t *testing.T) {
	// Build a sequence of exactly n bytes with BEL termination.
	build := func(n int) string {
		const head = "\x1b]7501;state=done:id=a:x="
		pad := n - len(head) - 1
		return head + strings.Repeat("a", pad) + "\x07"
	}
	c := feed(t, []byte(build(4096)))
	require.Len(t, c.reports, 1)

	c = feed(t, []byte(build(4097)))
	require.Empty(t, c.reports)
	require.Len(t, c.out.Bytes(), 4097)

	// ESC \ terminator counts too.
	st := strings.TrimSuffix(build(4096), "\x07") + "\x1b\\"
	c = feed(t, []byte(st))
	require.Empty(t, c.reports)
}

func TestScannerRecoversAfterAbort(t *testing.T) {
	in := "\x1b]7501;state=done\x18" + "junk" + string(wantReport.Encode())
	c := feed(t, []byte(in))
	require.Equal(t, in, c.out.String())
	require.Equal(t, []Report{wantReport}, c.reports)
}

func TestScannerESCRestartsOSC(t *testing.T) {
	in := "\x1b]7501;state=done\x1b" + string(wantReport.Encode())
	c := feed(t, []byte(in))
	require.Equal(t, in, c.out.String())
	require.Equal(t, []Report{wantReport}, c.reports)

	in = "\x1b]0;title\x1b" + string(wantReport.Encode())
	c = feed(t, []byte(in))
	require.Equal(t, []Report{wantReport}, c.reports)
}

func TestScannerQuery(t *testing.T) {
	in := []byte("a\x1b]7501;?\x1b\\b\x1b]7501;?\x07")
	c := feed(t, in)
	require.Equal(t, in, c.out.Bytes())
	require.Equal(t, 2, c.queries)
	require.Empty(t, c.reports)

	c = feed(t, []byte("\x1b]7501;? \x1b\\"))
	require.Zero(t, c.queries)
}

func TestScannerMultipleReports(t *testing.T) {
	a := Report{State: "working", Progress: 1}
	b := Report{State: "done", Progress: -1}
	in := append(a.Encode(), append([]byte("mid"), b.Encode()...)...)
	c := feed(t, in)
	require.Equal(t, in, c.out.Bytes())
	require.Equal(t, []Report{a, b}, c.reports)
}

func TestScannerOrdering(t *testing.T) {
	var out bytes.Buffer
	seq := wantReport.Encode()
	in := append(append([]byte("pre"), seq...), []byte("post")...)
	var seen int
	s := &Scanner{W: &out, OnReport: func(Report) {
		seen = out.Len()
	}}
	n, err := s.Write(in)
	require.NoError(t, err)
	require.Equal(t, len(in), n)
	require.Equal(t, len("pre")+len(seq), seen)
	require.Equal(t, in, out.Bytes())

	var qseen int
	out.Reset()
	s = &Scanner{W: &out, OnQuery: func() { qseen = out.Len() }}
	_, err = s.Write([]byte("a\x1b]7501;?\x07b"))
	require.NoError(t, err)
	require.Equal(t, len("a\x1b]7501;?\x07"), qseen)
}

func TestScannerNilCallbacks(t *testing.T) {
	var out bytes.Buffer
	s := &Scanner{W: &out}
	in := append(wantReport.Encode(), []byte("\x1b]7501;?\x07")...)
	_, err := s.Write(in)
	require.NoError(t, err)
	require.Equal(t, in, out.Bytes())
}

type failWriter struct{ limit int }

func (f *failWriter) Write(p []byte) (int, error) {
	if len(p) > f.limit {
		return f.limit, errors.New("boom")
	}
	f.limit -= len(p)
	return len(p), nil
}

func TestScannerWriteError(t *testing.T) {
	called := false
	s := &Scanner{W: &failWriter{limit: 3}, OnReport: func(Report) { called = true }}
	in := append([]byte("abc"), wantReport.Encode()...)
	n, err := s.Write(in)
	require.Error(t, err)
	require.Equal(t, 3, n)
	require.False(t, called)
}

func FuzzScanner(f *testing.F) {
	seeds := [][]byte{
		wantReport.Encode(),
		[]byte("text\x1b]7501;state=working:progress=5\x07more"),
		[]byte("\x1b]7501;?\x1b\\"),
		[]byte("\x1b]0;title\x07"),
		[]byte("\x1b]7501;state=done\x18\x07"),
		[]byte("\x1b]7501;state=done\x1b\x1b]7501;state=idle\x07"),
		[]byte("\x1b]7501;state=done:msg=!!!\x07"),
		[]byte("\x1b]7501;" + strings.Repeat("a", 5000)),
		[]byte("\x1b\x1b\x1b]\x1b\\\x07\x18\x1a"),
	}
	for _, s := range seeds {
		f.Add(s, uint16(3), uint16(11))
	}
	f.Fuzz(func(t *testing.T, data []byte, a, b uint16) {
		var out bytes.Buffer
		s := &Scanner{W: &out, OnReport: func(Report) {}, OnQuery: func() {}}
		i := int(a) % (len(data) + 1)
		j := int(b) % (len(data) + 1)
		if i > j {
			i, j = j, i
		}
		for _, chunk := range [][]byte{data[:i], data[i:j], data[j:]} {
			n, err := s.Write(chunk)
			require.NoError(t, err)
			require.Equal(t, len(chunk), n)
		}
		require.Equal(t, string(data), out.String())
	})
}

func TestQueryReplyIsNotAQuery(t *testing.T) {
	c := &capture{}
	s := c.scanner()
	n, err := s.Write([]byte(QueryReply))
	require.NoError(t, err)
	require.Equal(t, len(QueryReply), n)
	require.Empty(t, c.reports)
	require.Zero(t, c.queries)
	require.Equal(t, QueryReply, c.out.String())
}
