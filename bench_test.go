package osc7501

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

var (
	benchMinimal = []byte("state=idle")
	benchFull    = []byte("state=working:id=ci/build/42:progress=40:app=make:title=" +
		b64("Building osc7501") + ":msg=" + b64(strings.Repeat("compiling package ", 20)))

	// benchStream is terminal output with a report every ~2 KiB, split across
	// writes the way a PTY read loop would deliver it.
	benchStream = func() []byte {
		var b bytes.Buffer
		line := strings.Repeat("x", 79) + "\n"
		for range 64 {
			for range 25 {
				b.WriteString(line)
			}
			b.WriteString("\x1b]0;window title\x07")
			b.Write(Report{State: StateWorking, ID: "build", Progress: 50, App: "make", Msg: "hello"}.Encode())
		}
		return b.Bytes()
	}()
	benchPlain = bytes.Repeat([]byte(strings.Repeat("x", 79)+"\n"), 1600)
)

func BenchmarkParse(b *testing.B) {
	for _, bc := range []struct {
		name string
		body []byte
	}{{"minimal", benchMinimal}, {"full", benchFull}} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(bc.body)))
			for b.Loop() {
				if _, err := Parse(bc.body); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkScanner(b *testing.B) {
	for _, bc := range []struct {
		name string
		in   []byte
	}{{"plain", benchPlain}, {"reports", benchStream}} {
		b.Run(bc.name, func(b *testing.B) {
			s := &Scanner{W: io.Discard, OnReport: func(Report) {}}
			b.ReportAllocs()
			b.SetBytes(int64(len(bc.in)))
			for b.Loop() {
				for p := bc.in; len(p) > 0; {
					n := min(4096, len(p))
					if _, err := s.Write(p[:n]); err != nil {
						b.Fatal(err)
					}
					p = p[n:]
				}
			}
		})
	}
}

func BenchmarkStripper(b *testing.B) {
	for _, bc := range []struct {
		name string
		in   []byte
	}{{"plain", benchPlain}, {"reports", benchStream}} {
		b.Run(bc.name, func(b *testing.B) {
			var s Stripper
			b.ReportAllocs()
			b.SetBytes(int64(len(bc.in)))
			for b.Loop() {
				for p := bc.in; len(p) > 0; {
					n := min(4096, len(p))
					s.Strip(p[:n])
					p = p[n:]
				}
			}
		})
	}
}
