package osc7501

import "io"

type scanState uint8

const (
	stGround scanState = iota // outside any escape sequence
	stEsc                     // saw ESC
	stOSC                     // inside an OSC string
	stOSCEsc                  // saw ESC inside an OSC string
)

const prefix = "7501;"

// Scanner is an io.Writer that forwards every byte to W unchanged while
// recognising OSC 7501 reports and queries. Sequences may be split across
// Write calls at any byte. Callbacks run after all bytes up to and including
// the sequence terminator have been written to W.
type Scanner struct {
	W        io.Writer // downstream; receives all bytes unchanged
	OnReport func(Report)
	OnQuery  func()

	state   scanState
	matched bool   // OSC so far is a candidate 7501 sequence
	buf     []byte // OSC content after "ESC ]" while matched
}

// Write implements io.Writer. On a downstream error it returns the number of
// bytes of p accepted by W and that error.
func (s *Scanner) Write(p []byte) (int, error) {
	start := 0
	for i, c := range p {
		if !s.step(c) {
			continue
		}
		// Complete 7501 sequence ending at p[i]: forward first, then callback.
		if n, err := s.W.Write(p[start : i+1]); err != nil {
			return start + n, err
		}
		start = i + 1
		s.dispatch()
	}
	if start < len(p) {
		n, err := s.W.Write(p[start:])
		return start + n, err
	}
	return len(p), nil
}

// step advances the state machine and reports whether c completed a
// 7501 sequence (s.buf then holds its content, minus the terminator).
func (s *Scanner) step(c byte) bool {
	switch s.state {
	case stGround:
		if c == 0x1b {
			s.state = stEsc
		}
	case stEsc:
		switch c {
		case ']':
			s.startOSC()
		case 0x1b:
		default:
			s.state = stGround
		}
	case stOSC:
		switch c {
		case 0x07:
			return s.finish(1)
		case 0x1b:
			s.state = stOSCEsc
		case 0x18, 0x1a:
			s.state = stGround
		default:
			s.add(c)
		}
	case stOSCEsc:
		switch c {
		case '\\':
			return s.finish(2)
		case ']':
			s.startOSC()
		case 0x1b:
			s.state = stEsc // stray ESC aborts the OSC; this one may start a new sequence
		default:
			s.state = stGround
		}
	}
	return false
}

func (s *Scanner) startOSC() {
	s.state = stOSC
	s.matched = true
	s.buf = s.buf[:0]
}

func (s *Scanner) add(c byte) {
	if !s.matched {
		return
	}
	if i := len(s.buf); i < len(prefix) && c != prefix[i] {
		s.matched = false
		return
	}
	s.buf = append(s.buf, c)
	if 2+len(s.buf)+1 > maxSequence { // even a BEL terminator would exceed the cap
		s.matched = false
		s.state = stGround
	}
}

// finish ends the OSC with a terminator of stLen bytes and reports whether
// it was a complete 7501 sequence within the size cap.
func (s *Scanner) finish(stLen int) bool {
	s.state = stGround
	return s.matched && len(s.buf) >= len(prefix) && 2+len(s.buf)+stLen <= maxSequence
}

func (s *Scanner) dispatch() {
	body := s.buf[len(prefix):]
	if string(body) == "?" {
		if s.OnQuery != nil {
			s.OnQuery()
		}
		return
	}
	if s.OnReport == nil {
		return
	}
	if r, ok := Parse(body); ok {
		s.OnReport(r)
	}
}
