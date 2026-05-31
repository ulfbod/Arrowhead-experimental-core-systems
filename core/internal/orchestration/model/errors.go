package model

import "errors"

// ErrInterclouNotSupported is returned when ALLOW_INTERCLOUD or ONLY_INTERCLOUD is requested.
var ErrInterclouNotSupported = errors.New("intercloud orchestration is not supported")

// ErrOnlyExclusiveNotSupported is returned by SimpleStoreOrchestration when ONLY_EXCLUSIVE
// is requested; SimpleStore has no lock store and cannot implement exclusive-lock filtering.
var ErrOnlyExclusiveNotSupported = errors.New("ONLY_EXCLUSIVE orchestration flag is not supported in SimpleStoreOrchestration")
