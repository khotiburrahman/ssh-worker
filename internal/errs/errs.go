package errs

import "errors"

var (
	ErrNotConnected = errors.New("ssh: not connected")
	ErrUnavailable  = errors.New("ssh: unavailable")
	ErrCircuitOpen  = errors.New("qload: circuit open")
	ErrBusy         = errors.New("qload: busy")
	ErrShuttingDown = errors.New("app: shutting down")
)

// Retryable menandai error yang aman untuk dicoba ulang.
type Retryable struct{ Err error }

func (r *Retryable) Error() string { return r.Err.Error() }
func (r *Retryable) Unwrap() error { return r.Err }

func IsRetryable(err error) bool {
	var r *Retryable
	return errors.As(err, &r)
}

func Retry(err error) error {
	if err == nil {
		return nil
	}
	return &Retryable{Err: err}
}

// Coded = error + kode terstruktur.
type Coded struct {
	Code string
	Err  error
}

func (c *Coded) Error() string { return c.Err.Error() }
func (c *Coded) Unwrap() error { return c.Err }

func WithCode(code string, err error) error {
	if err == nil {
		return nil
	}
	return &Coded{Code: code, Err: err}
}

func CodeOf(err error) string {
	var c *Coded
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}