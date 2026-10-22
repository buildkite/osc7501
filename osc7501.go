// Package osc7501 implements the OSC 7501 "program status" terminal protocol
// (rev 0.4, https://www.superlogical.com/rex/docs/build/program-status).
//
// Programs use it to tell the terminal or host what they are doing (working,
// blocked, done, error and so on) with a report of the form
//
//	ESC ] 7501 ; key=value:key=value ST
//
// where ST is ESC \ or BEL. The package covers four uses:
//
//   - Emit reports with Reporter (or Report.Encode for the raw sequence).
//   - Decode a report body with Parse.
//   - Detect reports in a byte stream with Scanner, an io.Writer that forwards
//     every byte unchanged and calls OnReport and OnQuery. Answer a feature
//     query by writing QueryReply to the program's stdin.
//   - Remove reports from a byte stream with Stripper.
//
// Known limitation: Scanner and Stripper track only OSC framing, so an ESC ]
// that appears inside a DCS or APC string can be mis-framed.
package osc7501

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Protocol limits.
const (
	maxSequence   = 4096
	maxKeyLen     = 16
	maxIDLen      = 128
	maxIDLevels   = 8
	maxSegmentLen = 32
	maxTitleEnc   = 256
	maxTitleDec   = 192
	maxMsgEnc     = 2732
	maxMsgDec     = 2048

	// maxBody is the longest body that fits in a sequence: ESC ] "7501;" body BEL.
	maxBody = maxSequence - len("\x1b]7501;") - 1
)

// QueryReply is the feature-detection reply. It lists the rev 0.4 default
// states and kinds, so its body differs from the query: if the PTY echoes it
// back, Scanner does not mistake it for a new query (no reply loop).
const QueryReply = "\x1b]7501;?:states=idle,working,done,blocked,error:kinds=permission,question,auth\x1b\\"

// Errors returned by Parse, wrapped with the key or value that failed. Test
// for them with errors.Is.
var (
	ErrNoState      = errors.New("osc7501: missing state")
	ErrUnknownState = errors.New("osc7501: unknown state")
	ErrLimit        = errors.New("osc7501: limit exceeded")
	ErrInvalidID    = errors.New("osc7501: invalid id")
	ErrInvalidText  = errors.New("osc7501: invalid text")
)

// Report is a decoded program status report.
type Report struct {
	State, ID, Kind, App, Title, Msg string // Title and Msg are decoded
	Progress                         int    // 0-100, or -1 if indeterminate
}

// Parse decodes the body of a report (the bytes between "7501;" and ST).
// It returns an error if the whole report must be ignored. Every pair is checked
// against the protocol limits, so a report that breaks one is rejected even
// if a later duplicate key is valid. A body too long to fit in a 4096-byte
// sequence is rejected; Scanner enforces the exact limit with framing.
func Parse(body []byte) (Report, error) {
	if len(body) > maxBody {
		return Report{}, fmt.Errorf("%w: body is %d bytes, max %d", ErrLimit, len(body), maxBody)
	}
	// Known keys go straight into r; the last valid duplicate wins. Values are
	// converted to strings only for known keys, so unknown keys cost nothing.
	r := Report{Progress: -1}
	var kind, app string
	var progress []byte
	var hasProgress, hasApp bool
	for rest, more := body, true; more; {
		var pair []byte
		pair, rest, more = bytes.Cut(rest, []byte{':'})
		k, v, ok := bytes.Cut(pair, []byte{'='})
		if !ok {
			continue
		}
		k, v = bytes.TrimSpace(k), bytes.TrimSpace(v)
		if len(k) > maxKeyLen {
			return Report{}, fmt.Errorf("%w: key is %d bytes, max %d", ErrLimit, len(k), maxKeyLen)
		}
		if !validKey(k) || !validValue(v) {
			continue
		}
		var err error
		switch string(k) {
		case "state":
			r.State = string(v)
		case "id":
			r.ID = string(v)
			if !ValidID(r.ID) {
				err = fmt.Errorf("%w: %q", ErrInvalidID, v)
			}
		case "kind":
			kind = string(v)
		case "progress":
			progress, hasProgress = v, true
		case "app":
			if len(v) > maxSegmentLen {
				err = fmt.Errorf("%w: app is %d bytes, max %d", ErrLimit, len(v), maxSegmentLen)
			}
			app, hasApp = string(v), true
		case "title":
			r.Title, err = decodeText("title", v, maxTitleEnc, maxTitleDec)
		case "msg":
			r.Msg, err = decodeText("msg", v, maxMsgEnc, maxMsgDec)
		}
		if err != nil {
			return Report{}, err
		}
	}

	if r.State == "" {
		return Report{}, ErrNoState
	}
	if !validState(r.State) {
		return Report{}, fmt.Errorf("%w: %q", ErrUnknownState, r.State)
	}

	if r.State == StateBlocked && validKind(kind) {
		r.Kind = kind
	}

	if (r.State == StateWorking || r.State == StateBlocked) && hasProgress {
		r.Progress = parseProgress(progress)
	}

	if hasApp && ValidSegment(app) {
		r.App = app
	}
	return r, nil
}

// Encode returns the full report sequence terminated by ESC \.
// Keys are emitted in canonical order (state, id, kind, progress, app,
// title, msg). Title and Msg are sanitised and
// truncated to the protocol limits. Empty values are omitted, progress is
// emitted only for working or blocked with a value of 0 or more (clamped to
// 100), and kind only for blocked.
func (r Report) Encode() []byte {
	var b strings.Builder
	b.WriteString("\x1b]7501;state=" + r.State)
	add := func(k, v string) {
		if v != "" {
			b.WriteString(":" + k + "=" + v)
		}
	}
	add("id", r.ID)
	if r.State == StateBlocked {
		add("kind", r.Kind)
	}
	if (r.State == StateWorking || r.State == StateBlocked) && r.Progress >= 0 {
		add("progress", strconv.Itoa(min(r.Progress, 100)))
	}
	add("app", r.App)
	if t := cleanText(r.Title, maxTitleDec); t != "" {
		add("title", base64.StdEncoding.EncodeToString([]byte(t)))
	}
	if m := cleanText(r.Msg, maxMsgDec); m != "" {
		add("msg", base64.StdEncoding.EncodeToString([]byte(m)))
	}
	b.WriteString("\x1b\\")
	return []byte(b.String())
}

// cleanText replaces control characters with spaces and truncates s to at
// most max bytes on a rune boundary.
func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

func validKey(k []byte) bool {
	if len(k) == 0 || len(k) > maxKeyLen {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 'a' || k[i] > 'z' {
			return false
		}
	}
	return true
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func validValue(v []byte) bool {
	for i := 0; i < len(v); i++ {
		if !isAlnum(v[i]) && strings.IndexByte("_.,+/=-", v[i]) < 0 {
			return false
		}
	}
	return true
}

// ValidSegment reports whether s matches [A-Za-z0-9_.+-]{1,32}.
func ValidSegment(s string) bool {
	if s == "" || len(s) > maxSegmentLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isAlnum(s[i]) && strings.IndexByte("_.+-", s[i]) < 0 {
			return false
		}
	}
	return true
}

// ValidID reports whether id is 1-128 bytes of at most 8 '/'-separated
// segments, each valid per ValidSegment.
func ValidID(id string) bool {
	if id == "" || len(id) > maxIDLen {
		return false
	}
	for levels := 1; ; levels++ {
		seg, rest, more := strings.Cut(id, "/")
		if levels > maxIDLevels || !ValidSegment(seg) {
			return false
		}
		if !more {
			return true
		}
		id = rest
	}
}

func parseProgress(s []byte) int {
	if len(s) == 0 || len(s) > 3 {
		return -1
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	if n > 100 {
		return -1
	}
	return n
}

// decodeText decodes base64 (padding optional) UTF-8 text enforcing limits.
// key names the field in errors.
func decodeText(key string, s []byte, maxEnc, maxDec int) (string, error) {
	if len(s) > maxEnc {
		return "", fmt.Errorf("%w: %s is %d bytes encoded, max %d", ErrLimit, key, len(s), maxEnc)
	}
	enc := base64.RawStdEncoding
	if bytes.HasSuffix(s, []byte{'='}) {
		enc = base64.StdEncoding
	}
	raw := make([]byte, enc.DecodedLen(len(s)))
	n, err := enc.Decode(raw, s)
	if err != nil {
		return "", fmt.Errorf("%w: %s is not valid base64", ErrInvalidText, key)
	}
	raw = raw[:n]
	if len(raw) > maxDec {
		return "", fmt.Errorf("%w: %s is %d bytes decoded, max %d", ErrLimit, key, len(raw), maxDec)
	}
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidText, key)
	}
	for _, c := range string(raw) {
		if c < 0x20 || c == 0x7f || (c >= 0x80 && c <= 0x9f) {
			return "", fmt.Errorf("%w: %s contains control character %U", ErrInvalidText, key, c)
		}
	}
	return string(raw), nil
}
