package globalroledom

import "errors"

var (
	// ErrNotFound indicates the requested global role does not exist.
	ErrNotFound = errors.New("global role: not found")
	// ErrNameTaken indicates the role name is already in use.
	ErrNameTaken = errors.New("global role: name already in use")
	// ErrInvalidName indicates the provided role name is empty or invalid.
	ErrInvalidName = errors.New("global role: invalid name")
	// ErrReservedName indicates the role name is one the authorization layer
	// still treats as a privilege by NAME alone (SUPER_ADMIN / ADMIN / USER,
	// so sánh không phân biệt hoa thường). Đặt một vai mới mang tên ấy là đúc
	// ra `*` mà không cần một quyền nào trong bảng — trần cấp phát không nhìn
	// thấy đường ấy vì nó chỉ đọc bảng quyền.
	ErrReservedName = errors.New("global role: name is reserved by the legacy role-name fallback")
	// ErrHasAssignedUsers indicates the role cannot be deleted because one or
	// more users are still assigned to it (primary role FK or explicit assignment).
	ErrHasAssignedUsers = errors.New("global role: role has assigned users")
	// ErrPermissionCeilingExceeded indicates the caller attempted to create,
	// modify, or assign a role that grants a permission the caller does not
	// itself hold (a privilege-escalation attempt). Enforced as a grant ceiling:
	// a caller may never mint or hand out authority beyond their own.
	ErrPermissionCeilingExceeded = errors.New("global role: role grants permissions beyond the caller's own")
)
