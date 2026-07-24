package webconsole

import "testing"

func TestAccessRoleDefaultsToViewerAndOwnerIsOperator(t *testing.T) {
	for _, tc := range []struct{ owner, identity, want string }{
		{"owner@example.test", "owner@example.test", RoleOperator},
		{"OWNER@example.test", "owner@example.test", RoleOperator},
		{"owner@example.test", "viewer@example.test", RoleViewer},
		{"", "owner@example.test", RoleViewer},
		{"owner@example.test", "", RoleViewer},
	} {
		if got := roleForIdentity(tc.owner, tc.identity); got != tc.want {
			t.Fatalf("owner=%q identity=%q got=%q want=%q", tc.owner, tc.identity, got, tc.want)
		}
	}
}
