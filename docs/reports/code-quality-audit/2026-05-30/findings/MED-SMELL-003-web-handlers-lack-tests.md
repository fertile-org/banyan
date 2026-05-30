# [MED-SMELL-003] Web dashboard RPC handlers lack direct tests

**Severity**: Medium
**Category**: SMELL
**Component**: pkg/engine/grpc_handlers_web.go
**File(s)**: `pkg/engine/grpc_handlers_web.go:1-514`

## Description

The web dashboard RPC handlers in `grpc_handlers_web.go` are only tested indirectly via `TestGetDashboardData`. Direct tests for individual RPCs are missing:

- `GetClusterOverview`
- `ListAgents`
- `ListDeployments`
- `GetDeploymentDetail`
- `ListContainers`
- `ListEvents`
- `GetRecentLogs`

## Evidence

**Test coverage analysis**:
- `grpc_handlers_web.go` (514 lines) - NO dedicated test file
- Only `GetDashboardData` is tested via `TestGetDashboardData`
- All other web RPCs have no direct test coverage

**What the handlers do** (these are the lightweight per-page RPCs for the web dashboard):
```go
func (s *Server) GetClusterOverview(ctx context.Context, req *banyan.GetClusterOverviewRequest) (*banyan.GetClusterOverviewResponse, error)
func (s *Server) ListAgents(ctx context.Context, req *banyan.ListAgentsRequest) (*banyan.ListAgentsResponse, error)
func (s *Server) ListDeployments(ctx context.Context, req *banyan.ListDeploymentsRequest) (*banyan.ListDeploymentsResponse, error)
func (s *Server) GetDeploymentDetail(ctx context.Context, req *banyan.GetDeploymentDetailRequest) (*banyan.GetDeploymentDetailResponse, error)
func (s *Server) ListContainers(ctx context.Context, req *banyan.ListContainersRequest) (*banyan.ListContainersResponse, error)
func (s *Server) ListEvents(ctx context.Context, req *banyan.ListEventsRequest) (*banyan.ListEventsResponse, error)
func (s *Server) GetRecentLogs(ctx context.Context, req *banyan.GetRecentLogsRequest) (*banyan.GetRecentLogsResponse, error)
```

These are high-traffic handlers since the web dashboard calls them frequently.

## Impact

**Reliability impact**: Web dashboard users depend on these RPCs working correctly. Without tests, bugs in list filtering, error handling, or data aggregation could go undetected.

**User impact**: If `ListAgents` or `ListDeployments` breaks, the web dashboard becomes unusable for monitoring.

## Recommendation

1. **Add test file**: Create `pkg/engine/grpc_handlers_web_test.go`
2. **Cover all web RPCs** with tests for:
   - Happy path responses
   - Empty states (no agents, no deployments)
   - Error handling (store unavailable)
   - Pagination (if applicable)
   - Field selection / response completeness

3. **Test data filtering**: These handlers often filter by status or apply business logic — test that the filtering is correct.