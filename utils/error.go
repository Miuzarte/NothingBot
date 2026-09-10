package utils

import (
	"fmt"
)

func WrapErr(err error, detail any) error {
	if err == nil {
		return nil
	}
	return &error_{raw: err, detail: detail}
}

func UnwrapErr(err error) *error_ {
	if err == nil {
		return nil
	}
	if e, ok := err.(*error_); ok {
		return e
	}
	return &error_{raw: err}
}

type error_ struct {
	raw    error
	detail any
}

func (e *error_) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.detail != nil {
		return fmt.Sprintf("%s: %v", e.raw.Error(), e.detail)
	}
	return e.raw.Error()
}

func (e *error_) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.raw
}

func (e *error_) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	return e.raw == target
}
