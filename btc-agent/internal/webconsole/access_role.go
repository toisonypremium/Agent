package webconsole

import "strings"

const (
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

func roleForIdentity(owner, identity string) string {
	owner = strings.TrimSpace(strings.ToLower(owner))
	identity = strings.TrimSpace(strings.ToLower(identity))
	if owner != "" && identity != "" && owner == identity {
		return RoleOperator
	}
	return RoleViewer
}
