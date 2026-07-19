package worklogdom

import "errors"

// Sentinel domain errors for the worklog aggregate.
var (
	ErrWorklogNotFound         = errors.New("worklog: not found")
	ErrWorklogMinutesInvalid   = errors.New("worklog: minutes must be a positive integer")
	ErrWorklogTaskNotInProject = errors.New("worklog: task does not belong to this project")
)
