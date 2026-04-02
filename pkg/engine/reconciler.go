package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/metrics"
	"github.com/fertile-org/banyan/pkg/storage"
)

// Reconciler runs one reconciliation pass over the cluster state.
// Three implementations exist: AgentReconciler, ContainerReconciler, DeploymentReconciler.
// They are called in that order every 10 seconds by the engine loop.
type Reconciler interface {
	Reconcile(ctx context.Context) error
}

// ReconcilerDeps holds shared dependencies for all reconcilers.
type ReconcilerDeps struct {
	Store   storage.StateStore
	Events  EventLog
	Metrics *metrics.EngineMetricsRegistry
	Log     *logging.Logger
}

// EmitEvent adds an event and increments the Prometheus counter.
// Mirrors Engine.emitEvent() so reconcilers get the same behavior.
func (d *ReconcilerDeps) EmitEvent(eventType, message, severity string) {
	if d.Events != nil {
		d.Events.Add(Event{
			Timestamp: time.Now(),
			Type:      eventType,
			Message:   message,
			Severity:  severity,
		})
	}
	if d.Metrics != nil {
		d.Metrics.IncrementEvent(eventType)
	}
}

// reconcileLockTTL is the distributed lock TTL for per-deployment reconciliation.
const reconcileLockTTL = 30 * time.Second

// reconcileLockTimeout is the max time to wait for a lock before skipping a deployment.
const reconcileLockTimeout = 5 * time.Second

// RunReconciliation executes all reconcilers in order: Agent → Container → Deployment.
// Each reconciler runs independently; an error or panic in one does not block the others.
func RunReconciliation(ctx context.Context, reconcilers []Reconciler, log *logging.Logger) {
	for _, r := range reconcilers {
		func() {
			defer func() {
				if p := recover(); p != nil {
					log.Error("Reconciler panicked", "reconciler", fmt.Sprintf("%T", r), "panic", p)
				}
			}()
			if err := r.Reconcile(ctx); err != nil {
				log.Warn("Reconciler error", "reconciler", fmt.Sprintf("%T", r), "error", err)
			}
		}()
	}
}

// reconcileOnce prevents overlapping reconciliation runs when called from a goroutine.
var reconcileOnce sync.Mutex
