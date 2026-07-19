package componentdom

import "errors"

// Sentinel domain errors for the component aggregate.
var (
	ErrComponentNotFound    = errors.New("component: not found")
	ErrComponentNameInvalid = errors.New("component: name is empty or invalid")
	ErrComponentNameTaken   = errors.New("component: name already in use within project")
)
