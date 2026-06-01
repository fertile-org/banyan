package auth

// Permission represents an allowed resource+action pair.
type Permission struct {
	Resource string
	Action   string
}

// rolePermissions maps each role to its set of allowed permissions.
// Admin gets wildcard access. Deployer and viewer get explicit grants.
//
//	Role hierarchy (implicit, not inherited):
//	  admin    → everything
//	  deployer → deploy, manage deployments, read logs/status, read secrets
//	  viewer   → read-only access to deployments, containers, logs, status
var rolePermissions = map[string][]Permission{
	RoleAdmin: {
		{Resource: "*", Action: "*"},
	},
	RoleDeployer: {
		{Resource: "deployment", Action: "create"},
		{Resource: "deployment", Action: "read"},
		{Resource: "deployment", Action: "update"},
		{Resource: "deployment", Action: "delete"},
		{Resource: "deployment", Action: "scale"},
		{Resource: "container", Action: "read"},
		{Resource: "logs", Action: "read"},
		{Resource: "status", Action: "read"},
		{Resource: "secret", Action: "read"},
		{Resource: "user", Action: "update-self"},
	},
	RoleViewer: {
		{Resource: "deployment", Action: "read"},
		{Resource: "container", Action: "read"},
		{Resource: "logs", Action: "read"},
		{Resource: "status", Action: "read"},
		{Resource: "user", Action: "update-self"},
	},
}

// rpcPermissionMap maps gRPC full method names to the required resource+action.
// Methods not in this map are either bypassed (agent/auth RPCs) or denied by default.
var rpcPermissionMap = map[string]Permission{
	// CLI RPCs
	"/banyan.v1.EngineService/Deploy":    {Resource: "deployment", Action: "create"},
	"/banyan.v1.EngineService/Down":      {Resource: "deployment", Action: "delete"},
	"/banyan.v1.EngineService/Scale":     {Resource: "deployment", Action: "scale"},
	"/banyan.v1.EngineService/GetStatus": {Resource: "status", Action: "read"},
	"/banyan.v1.EngineService/GetLogs":   {Resource: "logs", Action: "read"},
	"/banyan.v1.EngineService/GetInfo":   {Resource: "status", Action: "read"},
	"/banyan.v1.EngineService/StopTask":  {Resource: "deployment", Action: "delete"},

	// Secret RPCs
	"/banyan.v1.EngineService/CreateSecret": {Resource: "secret", Action: "create"},
	"/banyan.v1.EngineService/ListSecrets":  {Resource: "secret", Action: "read"},
	"/banyan.v1.EngineService/GetSecret":    {Resource: "secret", Action: "read"},
	"/banyan.v1.EngineService/DeleteSecret": {Resource: "secret", Action: "delete"},

	// User management RPCs (admin only)
	"/banyan.v1.EngineService/CreateUser":     {Resource: "user", Action: "create"},
	"/banyan.v1.EngineService/ListUsers":      {Resource: "user", Action: "read"},
	"/banyan.v1.EngineService/DeleteUser":     {Resource: "user", Action: "delete"},
	"/banyan.v1.EngineService/UpdateUserRole": {Resource: "user", Action: "update"},
	"/banyan.v1.EngineService/ChangePassword": {Resource: "user", Action: "update-self"},

	// Dashboard RPCs (read-only)
	"/banyan.v1.EngineService/GetDashboardData":    {Resource: "status", Action: "read"},
	"/banyan.v1.EngineService/GetClusterOverview":  {Resource: "status", Action: "read"},
	"/banyan.v1.EngineService/ListAgents":          {Resource: "status", Action: "read"},
	"/banyan.v1.EngineService/ListDeployments":     {Resource: "deployment", Action: "read"},
	"/banyan.v1.EngineService/GetDeploymentDetail": {Resource: "deployment", Action: "read"},
	"/banyan.v1.EngineService/ListContainers":      {Resource: "container", Action: "read"},
	"/banyan.v1.EngineService/ListEvents":          {Resource: "status", Action: "read"},
	"/banyan.v1.EngineService/GetRecentLogs":       {Resource: "logs", Action: "read"},
}

// bypassMethods are RPCs that skip JWT authentication entirely.
// Agent RPCs are authenticated by WireGuard tunnel IP.
// Auth RPCs are unauthenticated by design.
var bypassMethods = map[string]bool{
	// Agent RPCs — authenticated by WireGuard tunnel IP
	"/banyan.v1.EngineService/Register":              true,
	"/banyan.v1.EngineService/Heartbeat":             true,
	"/banyan.v1.EngineService/PollTasks":             true,
	"/banyan.v1.EngineService/ReportTaskResult":      true,
	"/banyan.v1.EngineService/ReportContainerHealth": true,

	// Auth RPCs — unauthenticated (rate-limited instead)
	"/banyan.v1.EngineService/Login":        true,
	"/banyan.v1.EngineService/RefreshToken": true,

	// Health check — unauthenticated (used for connectivity probing)
	"/banyan.v1.EngineService/Health": true,
}

// IsBypassMethod returns true if the given gRPC method should skip JWT auth.
func IsBypassMethod(fullMethod string) bool {
	return bypassMethods[fullMethod]
}

// PermissionForMethod returns the required permission for a given gRPC method.
// Returns ok=false if the method is not in the permission map (should be denied by default).
func PermissionForMethod(fullMethod string) (Permission, bool) {
	p, ok := rpcPermissionMap[fullMethod]
	return p, ok
}

// RequiredRoleForPermission returns the minimum role name that has the given permission.
// Used for helpful error messages: "You need the 'deployer' role."
func RequiredRoleForPermission(resource, action string) string {
	// Check viewer first (most restrictive), then deployer, then admin
	for _, role := range []string{RoleViewer, RoleDeployer, RoleAdmin} {
		perms := rolePermissions[role]
		for _, p := range perms {
			if (p.Resource == resource || p.Resource == "*") &&
				(p.Action == action || p.Action == "*") {
				return role
			}
		}
	}
	return RoleAdmin // default to admin for unknown permissions
}
