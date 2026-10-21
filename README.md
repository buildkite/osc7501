# osc7501

Go implementation of OSC 7501, the "program status" terminal protocol ([spec rev 0.4](https://www.superlogical.com/rex/docs/build/program-status)). Programs use it to tell a terminal, CI runner or log viewer what they are doing: working (with progress), blocked on a prompt, done or failed. This package lets you emit those reports, detect them in a byte stream, and strip them from text output. It has no dependencies outside the standard library.

## About this package

This package was written with the help of an AI coding assistant (Claude Code by Anthropic). It is reviewed, tested (unit and fuzz tests) and maintained by a human maintainer, Mark Wolfe.

## Install

```bash
go get github.com/wolfeidau/osc7501
```

## Emit status

```go
r := osc7501.NewReporter(os.Stdout, "build")

_ = r.Working("compile", 40, "compiling") // progress 0-100; -1 = indeterminate
_ = r.Blocked("deploy", osc7501.KindPermission, "Approve deploy?")
_ = r.Done("deploy", "")
```

- `app` is set once in `NewReporter` and tagged on every report.
- Invalid state, id, app or kind returns an error wrapping `ErrInvalidReport`; nothing is written.
- Title and msg are sanitised (control characters become spaces) and truncated to the limits below.
- Progress is clamped to 100 and sent only for `working` and `blocked`. Kind is sent only for `blocked`.
- Each report is one `Write`, terminated with `ESC \`.
- Write `osc7501.Query` to detect whether the terminal supports OSC 7501; a supporting terminal replies on the program's stdin (so only when it runs on a terminal or PTY).

For full control, build a `Report` and call `Reporter.Report`, or `Report.Encode` for the raw bytes.

## Consume status

`Scanner` is an `io.Writer` that forwards every byte unchanged to `W` and calls back when it sees a report or a query. Sequences may be split across writes at any byte.

```go
s := &osc7501.Scanner{
	W: os.Stdout,
	OnReport: func(r osc7501.Report) {
		fmt.Fprintf(os.Stderr, "%s %s %d%%\n", r.State, r.ID, r.Progress)
	},
	OnQuery: func() {
		_, _ = io.WriteString(childStdin, osc7501.QueryReply) // PTY only
	},
}
_, _ = io.Copy(s, childOutput)
```

Callbacks run after the report bytes have been forwarded to `W`. `QueryReply` lists the default states and kinds; its body differs from the query, so an echoed reply does not trigger another query.

To decode a lone body (the bytes between `7501;` and the terminator), use `Parse`:

```go
r, err := osc7501.Parse([]byte("state=working:progress=50"))
```

A non-nil error means the whole report must be ignored. It wraps one of `ErrNoState`, `ErrUnknownState`, `ErrLimit`, `ErrInvalidID` or `ErrInvalidText` (test with `errors.Is`) and names the field that failed, for example `osc7501: limit exceeded: app is 33 bytes, max 32`.

## Strip status

Use `Stripper` to remove reports and queries from logs or text output. It frames sequences exactly as `Scanner` does.

```go
var s osc7501.Stripper
for chunk := range chunks {
	os.Stdout.Write(s.Strip(chunk))
}
os.Stdout.Write(s.Flush()) // bytes held for an unterminated sequence
```

Call `Flush` at the end of the stream. Other escape sequences pass through unchanged.

## Limits

| Item | Limit |
|------|-------|
| Sequence | 4096 bytes (`Parse` rejects bodies over 4088) |
| Key | 16 bytes |
| id | 128 bytes, 8 levels (`/`-separated) |
| id segment and app | `[A-Za-z0-9_.+-]{1,32}` |
| title | 256 bytes encoded, 192 decoded |
| msg | 2732 bytes encoded, 2048 decoded |

A report that breaks a limit is discarded whole, including when a later duplicate key is valid.

## Known limitations

`Scanner` and `Stripper` track only OSC framing. An `ESC ]` inside a DCS or APC string can be mis-framed.

## Stability

Pre-1.0. The package tracks spec rev 0.4. Breaking changes bump the minor version and are documented in release notes.

## Used by

[console-stream](https://github.com/wolfeidau/console-stream) streams process output and turns reports into `ProgramStatus` events.

## License

Apache-2.0. See [LICENSE](LICENSE).
