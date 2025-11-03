package rbac

type InvalidRoleNameError string

func (e InvalidRoleNameError) Error() string {
	return "invalid role name: " + string(e)
}

func NewErrInvalidRoleName(roleName string) error {
	return InvalidRoleNameError(roleName)
}
