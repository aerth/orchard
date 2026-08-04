// Package privateerr matey, it contains a `(error).Private() error` method!
package privateerr

import (
	"errors"

	"github.com/aerth/mostly/stackerr"
)

func Errorf(public string, f string, i ...any) error {
	e := stackerr.Errorf(f, i...)
	e.St = stackerr.GetFuncCallerInfo()
	return &E{underlying: e, public: public}
}

type E struct {
	underlying error
	public     string
}

func (e *E) Error() string {
	return e.public
}

func (e *E) Private() error {
	return e.underlying
}

func (e *E) Is(err error) bool {
	return e.Error() == e.public || errors.Is(err, e.underlying)
}

func (e *E) Unwrap() error {
	return e.underlying
}
