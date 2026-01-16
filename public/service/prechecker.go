package service

import (
	"github.com/warpstreamlabs/bento/internal/component"
)

type PreChecker interface {
	Check() error
}

type checkerWrapper struct {
	pc PreChecker
}

func newCheckerWrapper(pc PreChecker) component.Checkable {
	return &checkerWrapper{pc: pc}
}

func (w *checkerWrapper) Check() error {
	return w.pc.Check()
}
