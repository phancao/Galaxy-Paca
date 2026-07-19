package versiondom

import "errors"

// Sentinel domain errors for the version aggregate.
var (
	ErrVersionNotFound    = errors.New("version: not found")
	ErrVersionNameInvalid = errors.New("version: name is empty or invalid")
	ErrVersionNameTaken   = errors.New("version: name already in use within project")
)
