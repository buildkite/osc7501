package osc7501_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/wolfeidau/osc7501"
)

func ExampleReporter() {
	var buf bytes.Buffer
	r := osc7501.NewReporter(&buf, "build")

	_ = r.Working("compile", 40, "compiling")
	_ = r.Done("compile", "")

	fmt.Printf("%q\n", buf.String())
	// Output:
	// "\x1b]7501;state=working:id=compile:progress=40:app=build:msg=Y29tcGlsaW5n\x1b\\\x1b]7501;state=done:id=compile:app=build\x1b\\"
}

func ExampleParse() {
	r, err := osc7501.Parse([]byte("state=blocked:id=deploy:kind=permission:msg=QXBwcm92ZT8"))
	fmt.Println(err, r.State, r.ID, r.Kind, r.Msg, r.Progress)

	_, err = osc7501.Parse([]byte("state=done:app=" + strings.Repeat("a", 33)))
	fmt.Println(errors.Is(err, osc7501.ErrLimit), err)
	// Output:
	// <nil> blocked deploy permission Approve? -1
	// true osc7501: limit exceeded: app is 33 bytes, max 32
}

func ExampleScanner() {
	var out bytes.Buffer
	s := &osc7501.Scanner{
		W: &out,
		OnReport: func(r osc7501.Report) {
			fmt.Println("report:", r.State, r.ID, r.Progress)
		},
	}

	// The report is split across two writes.
	_, _ = s.Write([]byte("start\n\x1b]7501;state=working:id=b"))
	_, _ = s.Write([]byte("uild:progress=50\x1b\\end\n"))

	fmt.Printf("downstream: %q\n", out.String())
	// Output:
	// report: working build 50
	// downstream: "start\n\x1b]7501;state=working:id=build:progress=50\x1b\\end\n"
}

func ExampleStripper() {
	var s osc7501.Stripper

	fmt.Printf("%q\n", s.Strip([]byte("before \x1b]7501;state=done\x1b\\after\n")))
	fmt.Printf("%q\n", s.Strip([]byte("partial \x1b]7501;sta")))
	fmt.Printf("%q\n", s.Flush())
	// Output:
	// "before after\n"
	// "partial "
	// "\x1b]7501;sta"
}
