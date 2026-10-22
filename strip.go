package osc7501

import "bytes"

// Stripper removes complete OSC 7501 sequences (reports and queries) from a
// byte stream. It frames sequences exactly as Scanner does, so it removes
// precisely the sequences Scanner would report. State persists across Strip
// calls, so sequences may be split at any byte.
//
// Bytes from an ESC that may begin a 7501 sequence are held back until the
// sequence resolves. If it aborts, is not 7501, or exceeds the size cap, the
// held bytes are emitted unchanged and in order. Everything else, including
// other OSC sequences, passes through unchanged.
type Stripper struct {
	state   scanState
	matched bool   // current OSC is still a candidate 7501 sequence
	n       int    // OSC content length so far while matched
	held    []byte // bytes withheld from output
}

// Strip returns p with complete OSC 7501 sequences removed. The result may
// alias p.
func (s *Stripper) Strip(p []byte) []byte {
	if s.state == stGround && len(s.held) == 0 && bytes.IndexByte(p, 0x1b) < 0 {
		return p
	}
	out := make([]byte, 0, len(p)+len(s.held))
	for i := 0; i < len(p); i++ {
		if s.state == stGround { // copy plain text up to the next ESC in one go
			j := bytes.IndexByte(p[i:], 0x1b)
			if j < 0 {
				return append(out, p[i:]...)
			}
			out = append(out, p[i:i+j]...)
			i += j
		}
		out = s.step(out, p[i])
	}
	return out
}

// Flush returns any bytes still held (for example an unterminated sequence at
// the end of the stream) and resets the Stripper.
func (s *Stripper) Flush() []byte {
	var out []byte
	if len(s.held) > 0 {
		out = append(out, s.held...)
	}
	*s = Stripper{held: s.held[:0]}
	return out
}

func (s *Stripper) release(out []byte) []byte {
	out = append(out, s.held...)
	s.held = s.held[:0]
	return out
}

func (s *Stripper) startOSC() {
	s.state = stOSC
	s.matched = true
	s.n = 0
}

func (s *Stripper) step(out []byte, c byte) []byte {
	switch s.state {
	case stGround:
		if c == 0x1b {
			s.state = stEsc
			s.held = append(s.held[:0], c)
		} else {
			out = append(out, c)
		}
	case stEsc:
		switch c {
		case ']':
			s.held = append(s.held, c)
			s.startOSC()
		case 0x1b:
			out = s.release(out) // previous ESC starts nothing
			s.held = append(s.held, c)
		default:
			out = s.release(out)
			out = append(out, c)
			s.state = stGround
		}
	case stOSC:
		out = s.stepOSC(out, c)
	case stOSCEsc:
		switch c {
		case '\\':
			return s.finish(out, c, 2)
		case ']':
			// ESC ] restarts: the aborted candidate is emitted, the ESC begins anew.
			last := len(s.held) - 1
			out = append(out, s.held[:last]...)
			s.held = append(s.held[:0], 0x1b, ']')
			s.startOSC()
		case 0x1b:
			out = s.release(out)
			s.held = append(s.held, c)
			s.state = stEsc
		default:
			out = s.release(out)
			out = append(out, c)
			s.state = stGround
		}
	}
	return out
}

func (s *Stripper) stepOSC(out []byte, c byte) []byte {
	switch c {
	case 0x07:
		return s.finish(out, c, 1)
	case 0x1b:
		s.held = append(s.held, c)
		s.state = stOSCEsc
	case 0x18, 0x1a:
		out = s.release(out)
		out = append(out, c)
		s.state = stGround
	default:
		if !s.matched {
			return append(out, c)
		}
		s.held = append(s.held, c)
		if s.n < len(prefix) && c != prefix[s.n] {
			s.matched = false
			return s.release(out)
		}
		s.n++
		if 2+s.n+1 > maxSequence {
			s.matched = false
			s.state = stGround
			return s.release(out)
		}
	}
	return out
}

// finish ends the OSC with a terminator of stLen bytes ending in c.
func (s *Stripper) finish(out []byte, c byte, stLen int) []byte {
	s.state = stGround
	if s.matched && s.n >= len(prefix) && 2+s.n+stLen <= maxSequence {
		s.held = s.held[:0] // complete 7501 sequence: drop it
		return out
	}
	out = s.release(out)
	return append(out, c)
}
