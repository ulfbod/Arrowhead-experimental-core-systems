package model

import "errors"

// ErrInterclouNotSupported is returned when ALLOW_INTERCLOUD or ONLY_INTERCLOUD is requested.
var ErrInterclouNotSupported = errors.New("intercloud orchestration is not supported")
