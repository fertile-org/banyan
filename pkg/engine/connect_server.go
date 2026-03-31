// connect_server.go provides a connect-go HTTP API server that wraps the existing
// gRPC server. This gives the web dashboard an HTTP/JSON API without touching
// the existing agent/CLI gRPC transport.
package engine

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb/banyanpbconnect"
)

// connectAdapter wraps engineGRPCServer to implement the connect-go EngineServiceHandler interface.
// This provides an HTTP/JSON API for the web dashboard while keeping the existing gRPC server
// untouched for agents and CLI.
type connectAdapter struct {
	banyanpbconnect.UnimplementedEngineServiceHandler
	srv *engineGRPCServer
}

// --- Agent RPCs (forwarded for completeness, dashboard doesn't use these) ---

func (a *connectAdapter) Register(ctx context.Context, req *connect.Request[banyanpb.RegisterRequest]) (*connect.Response[banyanpb.RegisterResponse], error) {
	resp, err := a.srv.Register(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) Heartbeat(ctx context.Context, req *connect.Request[banyanpb.HeartbeatRequest]) (*connect.Response[banyanpb.HeartbeatResponse], error) {
	resp, err := a.srv.Heartbeat(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) PollTasks(ctx context.Context, req *connect.Request[banyanpb.PollTasksRequest]) (*connect.Response[banyanpb.PollTasksResponse], error) {
	resp, err := a.srv.PollTasks(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) ReportTaskResult(ctx context.Context, req *connect.Request[banyanpb.ReportTaskResultRequest]) (*connect.Response[banyanpb.ReportTaskResultResponse], error) {
	resp, err := a.srv.ReportTaskResult(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) ReportContainerHealth(ctx context.Context, req *connect.Request[banyanpb.ReportContainerHealthRequest]) (*connect.Response[banyanpb.ReportContainerHealthResponse], error) {
	resp, err := a.srv.ReportContainerHealth(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// --- CLI RPCs ---

func (a *connectAdapter) Deploy(ctx context.Context, req *connect.Request[banyanpb.DeployRPCRequest]) (*connect.Response[banyanpb.DeployRPCResponse], error) {
	resp, err := a.srv.Deploy(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) Down(ctx context.Context, req *connect.Request[banyanpb.DownRPCRequest]) (*connect.Response[banyanpb.DownRPCResponse], error) {
	resp, err := a.srv.Down(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) GetStatus(ctx context.Context, req *connect.Request[banyanpb.GetStatusRequest]) (*connect.Response[banyanpb.GetStatusResponse], error) {
	resp, err := a.srv.GetStatus(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) GetLogs(ctx context.Context, req *connect.Request[banyanpb.GetLogsRequest], stream *connect.ServerStream[banyanpb.GetLogsResponse]) error {
	if req.Msg.ContainerName == "" {
		return connect.NewError(connect.CodeInvalidArgument, nil)
	}

	task, node, err := a.srv.findContainerAgent(ctx, req.Msg.ContainerName)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if node.APIAddress == "" {
		return connect.NewError(connect.CodeUnavailable, nil)
	}

	reader, err := streamAgentLogs(ctx, node.APIAddress, task.ContainerName, req.Msg.Follow, req.Msg.Tail)
	if err != nil {
		return connect.NewError(connect.CodeUnavailable, err)
	}
	defer reader.Close()

	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if sendErr := stream.Send(&banyanpb.GetLogsResponse{Data: data}); sendErr != nil {
				return sendErr
			}
		}
		if readErr != nil {
			return nil // EOF or context cancel
		}
	}
}

func (a *connectAdapter) GetInfo(ctx context.Context, req *connect.Request[banyanpb.GetInfoRequest]) (*connect.Response[banyanpb.GetInfoResponse], error) {
	resp, err := a.srv.GetInfo(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) Health(ctx context.Context, req *connect.Request[banyanpb.HealthRequest]) (*connect.Response[banyanpb.HealthResponse], error) {
	resp, err := a.srv.Health(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) Scale(ctx context.Context, req *connect.Request[banyanpb.ScaleRequest]) (*connect.Response[banyanpb.ScaleResponse], error) {
	resp, err := a.srv.Scale(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) StopTask(ctx context.Context, req *connect.Request[banyanpb.StopTaskRequest]) (*connect.Response[banyanpb.StopTaskResponse], error) {
	resp, err := a.srv.StopTask(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// --- Dashboard RPCs ---

func (a *connectAdapter) GetDashboardData(ctx context.Context, req *connect.Request[banyanpb.GetDashboardDataRequest]) (*connect.Response[banyanpb.GetDashboardDataResponse], error) {
	resp, err := a.srv.GetDashboardData(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// --- Secret RPCs ---

func (a *connectAdapter) CreateSecret(ctx context.Context, req *connect.Request[banyanpb.CreateSecretRequest]) (*connect.Response[banyanpb.CreateSecretResponse], error) {
	resp, err := a.srv.CreateSecret(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) ListSecrets(ctx context.Context, req *connect.Request[banyanpb.ListSecretsRequest]) (*connect.Response[banyanpb.ListSecretsResponse], error) {
	resp, err := a.srv.ListSecrets(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) GetSecret(ctx context.Context, req *connect.Request[banyanpb.GetSecretRequest]) (*connect.Response[banyanpb.GetSecretResponse], error) {
	resp, err := a.srv.GetSecret(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (a *connectAdapter) DeleteSecret(ctx context.Context, req *connect.Request[banyanpb.DeleteSecretRequest]) (*connect.Response[banyanpb.DeleteSecretResponse], error) {
	resp, err := a.srv.DeleteSecret(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// corsMiddleware adds CORS headers for browser-based dashboard access.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, Grpc-Timeout, X-Grpc-Web, X-User-Agent")
		w.Header().Set("Access-Control-Expose-Headers", "Grpc-Status, Grpc-Message, Grpc-Status-Details-Bin")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// startConnectAPI starts the connect-go HTTP API server alongside the existing gRPC server.
// It provides an HTTP/JSON API for the web dashboard.
func startConnectAPI(ctx context.Context, srv *engineGRPCServer, port string) error {
	adapter := &connectAdapter{srv: srv}
	mux := http.NewServeMux()
	path, handler := banyanpbconnect.NewEngineServiceHandler(adapter)
	mux.Handle(path, corsMiddleware(handler))

	httpServer := &http.Server{
		Addr:              "0.0.0.0:" + port,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srv.logger().Error("Connect API server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	srv.logger().Info("Connect API server started", "port", port)
	return nil
}
