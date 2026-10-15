package osc7501

import (
	"errors"
	"fmt"
	"io"
)

// Report states.
const (
	StateIdle    = "idle"
	StateWorking = "working"
	StateDone    = "done"
	StateBlocked = "blocked"
	StateError   = "error"
	StateClear   = "clear"
)

// Blocked kinds.
const (
	KindPermission = "permission"
	KindQuestion   = "question"
	KindAuth       = "auth"
)

// Query is the feature query. A terminal that supports OSC 7501 replies on
// the program's stdin.
const Query = "\x1b]7501;?\x1b\\"

// ErrInvalidReport is wrapped by errors from Report.Validate and Reporter.
var ErrInvalidReport = errors.New("osc7501: invalid report")

func validState(s string) bool {
	switch s {
	case StateIdle, StateWorking, StateDone, StateBlocked, StateError, StateClear:
		return true
	}
	return false
}

func validKind(k string) bool {
	switch k {
	case KindPermission, KindQuestion, KindAuth:
		return true
	}
	return false
}

// Validate reports whether r can be encoded as a valid report.
func (r Report) Validate() error {
	switch {
	case !validState(r.State):
		return fmt.Errorf("%w: state %q", ErrInvalidReport, r.State)
	case r.ID != "" && !ValidID(r.ID):
		return fmt.Errorf("%w: id %q", ErrInvalidReport, r.ID)
	case r.App != "" && !ValidSegment(r.App):
		return fmt.Errorf("%w: app %q", ErrInvalidReport, r.App)
	case r.Kind != "" && !validKind(r.Kind):
		return fmt.Errorf("%w: kind %q", ErrInvalidReport, r.Kind)
	}
	return nil
}

// Reporter writes OSC 7501 reports for one program.
type Reporter struct {
	w   io.Writer
	app string
}

// NewReporter returns a Reporter that writes to w and tags reports with app.
func NewReporter(w io.Writer, app string) *Reporter {
	return &Reporter{w: w, app: app}
}

// Report validates rep and writes it in a single Write. App defaults to the
// Reporter's app. Nothing is written if rep is invalid.
func (r *Reporter) Report(rep Report) error {
	if rep.App == "" {
		rep.App = r.app
	}
	if err := rep.Validate(); err != nil {
		return err
	}
	_, err := r.w.Write(rep.Encode())
	return err
}

// Working reports id as working. A progress of -1 is indeterminate.
func (r *Reporter) Working(id string, progress int, msg string) error {
	return r.Report(Report{State: StateWorking, ID: id, Progress: progress, Msg: msg})
}

// Blocked reports id as waiting for a person.
func (r *Reporter) Blocked(id, kind, msg string) error {
	return r.Report(Report{State: StateBlocked, ID: id, Kind: kind, Progress: -1, Msg: msg})
}

// Done reports id as finished.
func (r *Reporter) Done(id, msg string) error {
	return r.Report(Report{State: StateDone, ID: id, Progress: -1, Msg: msg})
}

// Failed reports id as failed.
func (r *Reporter) Failed(id, msg string) error {
	return r.Report(Report{State: StateError, ID: id, Progress: -1, Msg: msg})
}

// Idle reports id as idle.
func (r *Reporter) Idle(id string) error {
	return r.Report(Report{State: StateIdle, ID: id, Progress: -1})
}

// Clear removes the record for id; an empty id clears all records from this
// program.
func (r *Reporter) Clear(id string) error {
	return r.Report(Report{State: StateClear, ID: id, Progress: -1})
}
