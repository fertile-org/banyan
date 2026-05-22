package auth

import "testing"

func TestIsBypassMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"/banyan.v1.EngineService/Register", true},
		{"/banyan.v1.EngineService/Heartbeat", true},
		{"/banyan.v1.EngineService/PollTasks", true},
		{"/banyan.v1.EngineService/ReportTaskResult", true},
		{"/banyan.v1.EngineService/ReportContainerHealth", true},
		{"/banyan.v1.EngineService/Login", true},
		{"/banyan.v1.EngineService/RefreshToken", true},
		{"/banyan.v1.EngineService/Health", true},
		{"/banyan.v1.EngineService/Deploy", false},
		{"/banyan.v1.EngineService/GetStatus", false},
		{"/banyan.v1.EngineService/CreateSecret", false},
		{"/banyan.v1.EngineService/CreateUser", false},
		{"/unknown/Method", false},
	}
	for _, tt := range tests {
		if got := IsBypassMethod(tt.method); got != tt.want {
			t.Errorf("IsBypassMethod(%q) = %v, want %v", tt.method, got, tt.want)
		}
	}
}

func TestPermissionForMethod(t *testing.T) {
	tests := []struct {
		method       string
		wantResource string
		wantAction   string
		wantOK       bool
	}{
		{"/banyan.v1.EngineService/Deploy", "deployment", "create", true},
		{"/banyan.v1.EngineService/Down", "deployment", "delete", true},
		{"/banyan.v1.EngineService/Scale", "deployment", "scale", true},
		{"/banyan.v1.EngineService/GetStatus", "status", "read", true},
		{"/banyan.v1.EngineService/GetLogs", "logs", "read", true},
		{"/banyan.v1.EngineService/CreateSecret", "secret", "create", true},
		{"/banyan.v1.EngineService/GetSecret", "secret", "read", true},
		{"/banyan.v1.EngineService/DeleteSecret", "secret", "delete", true},
		{"/banyan.v1.EngineService/CreateUser", "user", "create", true},
		{"/banyan.v1.EngineService/ListUsers", "user", "read", true},
		{"/banyan.v1.EngineService/DeleteUser", "user", "delete", true},
		{"/banyan.v1.EngineService/UpdateUserRole", "user", "update", true},
		{"/banyan.v1.EngineService/ChangePassword", "user", "update-self", true},
		{"/banyan.v1.EngineService/GetDashboardData", "status", "read", true},
		{"/banyan.v1.EngineService/ListContainers", "container", "read", true},
		{"/banyan.v1.EngineService/GetRecentLogs", "logs", "read", true},
		{"/unknown/Method", "", "", false},
	}
	for _, tt := range tests {
		perm, ok := PermissionForMethod(tt.method)
		if ok != tt.wantOK {
			t.Errorf("PermissionForMethod(%q) ok = %v, want %v", tt.method, ok, tt.wantOK)
			continue
		}
		if ok {
			if perm.Resource != tt.wantResource {
				t.Errorf("PermissionForMethod(%q) resource = %q, want %q", tt.method, perm.Resource, tt.wantResource)
			}
			if perm.Action != tt.wantAction {
				t.Errorf("PermissionForMethod(%q) action = %q, want %q", tt.method, perm.Action, tt.wantAction)
			}
		}
	}
}

func TestRolePermissions_AdminHasWildcard(t *testing.T) {
	perms := rolePermissions[RoleAdmin]
	if len(perms) != 1 {
		t.Fatalf("expected admin to have 1 wildcard permission, got %d", len(perms))
	}
	if perms[0].Resource != "*" || perms[0].Action != "*" {
		t.Errorf("expected admin wildcard *:*, got %s:%s", perms[0].Resource, perms[0].Action)
	}
}

func TestRolePermissions_DeployerCanDeploy(t *testing.T) {
	perms := rolePermissions[RoleDeployer]
	found := false
	for _, p := range perms {
		if p.Resource == "deployment" && p.Action == "create" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected deployer to have deployment:create permission")
	}
}

func TestRolePermissions_ViewerCannotDeploy(t *testing.T) {
	perms := rolePermissions[RoleViewer]
	for _, p := range perms {
		if p.Resource == "deployment" && p.Action == "create" {
			t.Error("viewer should NOT have deployment:create permission")
		}
	}
}

func TestRolePermissions_ViewerCanRead(t *testing.T) {
	perms := rolePermissions[RoleViewer]
	found := false
	for _, p := range perms {
		if p.Resource == "status" && p.Action == "read" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected viewer to have status:read permission")
	}
}

func TestRolePermissions_AllRolesCanChangeSelfPassword(t *testing.T) {
	for _, role := range []string{RoleDeployer, RoleViewer} {
		perms := rolePermissions[role]
		found := false
		for _, p := range perms {
			if p.Resource == "user" && p.Action == "update-self" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to have user:update-self permission", role)
		}
	}
}

func TestRequiredRoleForPermission(t *testing.T) {
	tests := []struct {
		resource string
		action   string
		want     string
	}{
		{"status", "read", RoleViewer},
		{"deployment", "read", RoleViewer},
		{"deployment", "create", RoleDeployer},
		{"deployment", "delete", RoleDeployer},
		{"secret", "read", RoleDeployer},
		{"secret", "create", RoleAdmin},
		{"secret", "delete", RoleAdmin},
		{"user", "create", RoleAdmin},
		{"user", "delete", RoleAdmin},
		{"user", "update-self", RoleViewer},
		{"unknown", "unknown", RoleAdmin}, // defaults to admin
	}
	for _, tt := range tests {
		got := RequiredRoleForPermission(tt.resource, tt.action)
		if got != tt.want {
			t.Errorf("RequiredRoleForPermission(%q, %q) = %q, want %q", tt.resource, tt.action, got, tt.want)
		}
	}
}

func TestAllMappedMethodsHavePermissions(t *testing.T) {
	// Every method in rpcPermissionMap should have a valid resource/action
	for method, perm := range rpcPermissionMap {
		if perm.Resource == "" {
			t.Errorf("method %q has empty resource", method)
		}
		if perm.Action == "" {
			t.Errorf("method %q has empty action", method)
		}
	}
}

func TestBypassAndPermissionMapNoOverlap(t *testing.T) {
	// No method should be in both bypass and permission maps
	for method := range bypassMethods {
		if _, ok := rpcPermissionMap[method]; ok {
			t.Errorf("method %q is in both bypass and permission maps", method)
		}
	}
}
