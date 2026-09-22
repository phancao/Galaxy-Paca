package projectdom

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Sentinel domain errors for the project aggregate.
var (
	ErrNotFound           = errors.New("project: not found")
	ErrNameTaken          = errors.New("project: name already in use")
	ErrNameInvalid        = errors.New("project: name is empty or invalid")
	ErrPrefixInvalid      = errors.New("project: task ID prefix must be 1–10 uppercase letters/digits")
	ErrMemberAlreadyAdded = errors.New("project: user is already a member")
	ErrMemberNotFound     = errors.New("project: member not found")
	ErrRoleNotFound       = errors.New("project: role not found")
	ErrRoleNameTaken      = errors.New("project: role name already in use")
	ErrRoleNameInvalid    = errors.New("project: role name is empty or invalid")
	ErrRoleHasMembers     = errors.New("project: role still has members assigned")
	// ErrPermissionCeilingExceeded indicates the caller attempted to grant a
	// member a project role whose permissions exceed the caller's own effective
	// permissions for the project (a privilege-escalation attempt via the
	// shared role templates, e.g. PROJECT_OWNER). Enforced as a grant ceiling.
	ErrPermissionCeilingExceeded = errors.New("project: role grants permissions beyond the caller's own")
	// ErrRolePermissionsInvalid indicates a project role was created or updated
	// with one or more permission keys that exist in no vocabulary — neither the
	// host's built-in permission set (internal/platform/authz/permissions.go) nor
	// the custom permissions declared by an installed plugin.
	//
	// Such a key is not merely cosmetic: the PACA-4 grant ceiling
	// (enforceRoleGrantCeiling) requires an assigning caller to COVER every
	// permission the role grants, and nobody can hold a key that does not exist.
	// A role carrying one is therefore unassignable by anyone but a "*" holder —
	// dead from birth, silently. Rejecting at write time is the cheap end.
	ErrRolePermissionsInvalid = errors.New("project: role contains unknown permission keys")
)

// UnknownPermissionsError carries the offending keys so the API can name them
// instead of returning a blank 400. It unwraps to ErrRolePermissionsInvalid so
// errors.Is keeps working for callers that only care about the class.
type UnknownPermissionsError struct {
	// Keys are the permission keys that matched no known vocabulary, sorted.
	Keys []string
}

// NewUnknownPermissionsError builds the error from an unsorted key list.
func NewUnknownPermissionsError(keys []string) *UnknownPermissionsError {
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	sort.Strings(sorted)
	return &UnknownPermissionsError{Keys: sorted}
}

func (e *UnknownPermissionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrRolePermissionsInvalid.Error(), strings.Join(e.Keys, ", "))
}

// Unwrap lets errors.Is(err, ErrRolePermissionsInvalid) succeed.
func (e *UnknownPermissionsError) Unwrap() error { return ErrRolePermissionsInvalid }
