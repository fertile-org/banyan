# Task Executor - Detailed Design

## Overview

The Task Executor is the central coordination component of the Agent, responsible for receiving tasks from the Engine, managing task execution, and routing work to the appropriate agent components (Container Runtime, Network Node, Security Executor). It acts as the entry point for all work dispatched to the Agent.

## Responsibilities

1. **Task Reception** - Receive tasks from Engine via gRPC streaming
2. **Task Routing** - Route tasks to appropriate components based on type
3. **Task Lifecycle** - Manage task states (pending, running, completed, failed)
4. **Concurrency Control** - Manage worker pools and task parallelism
5. **Result Reporting** - Report task results back to Engine
6. **Retry Management** - Handle task retries with backoff

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            Task Executor                                  │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │                       Driving Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  gRPC Stream     │  │   Task Queue     │  │   Local CLI       │ │ │
│  │  │  (from Engine)   │  │   (persistent)   │  │   (debug)         │ │ │
│  │  └────────┬─────────┘  └────────┬─────────┘  └─────────┬─────────┘ │ │
│  └───────────┼─────────────────────┼─────────────────────┼───────────┘ │
│              │                     │                     │              │
│              └─────────────────────┴─────────────────────┘              │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Inbound Ports                                 │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                  TaskExecutorService                         │   │ │
│  │  │  - SubmitTask(task) -> TaskResult                           │   │ │
│  │  │  - CancelTask(taskID) -> error                              │   │ │
│  │  │  - GetTaskStatus(taskID) -> TaskStatus                      │   │ │
│  │  │  - ListTasks(filter) -> []Task                              │   │ │
│  │  │  - SubscribeTaskEvents() -> EventStream                     │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                  WorkerPoolService                           │   │ │
│  │  │  - GetPoolStats() -> PoolStats                              │   │ │
│  │  │  - ScalePool(size) -> error                                 │   │ │
│  │  │  - DrainPool() -> error                                     │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                          Use Cases                                   │ │
│  │  ┌─────────────────┐ ┌─────────────────┐ ┌───────────────────────┐ │ │
│  │  │ TaskDispatch    │ │   TaskRouter    │ │  WorkerManagement     │ │ │
│  │  │   UseCase       │ │    UseCase      │ │      UseCase          │ │ │
│  │  │ - Submit        │ │ - Route         │ │ - Scale               │ │ │
│  │  │ - Cancel        │ │ - Validate      │ │ - Monitor             │ │ │
│  │  │ - Retry         │ │ - Priority      │ │ - Drain               │ │ │
│  │  └─────────────────┘ └─────────────────┘ └───────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                         Domain Layer                                 │ │
│  │  ┌────────────────────────────────────────────────────────────────┐ │ │
│  │  │  Entities: Task, TaskResult, Worker, WorkerPool               │ │ │
│  │  │  Value Objects: TaskID, TaskType, TaskPriority, TaskState     │ │ │
│  │  │  Domain Logic: State machine, Priority queue, Retry policy    │ │ │
│  │  └────────────────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Outbound Ports                                │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │ ContainerService  │ │  NetworkService   │ │ SecurityService   │ │ │
│  │  │ (Container Runtime)│ │  (Network Node)  │ │ (Security Exec)   │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │   TaskStore       │ │  ResultReporter   │ │   EventEmitter    │ │ │
│  │  │   (persistence)   │ │  (to Engine)      │ │   (events)        │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Driven Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  Container       │  │   Network        │  │   Security        │ │ │
│  │  │  Runtime Client  │  │   Node Client    │  │   Executor Client │ │ │
│  │  └──────────────────┘  └──────────────────┘  └───────────────────┘ │ │
│  │  ┌──────────────────┐  ┌──────────────────┐                        │ │
│  │  │  gRPC Result     │  │   BoltDB Store   │                        │ │
│  │  │  Reporter        │  │   (task queue)   │                        │ │
│  │  └──────────────────┘  └──────────────────┘                        │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// Task represents a unit of work to be executed
type Task struct {
    ID          TaskID
    Type        TaskType
    Priority    TaskPriority
    Payload     TaskPayload
    State       TaskState
    RetryCount  int
    MaxRetries  int
    Timeout     time.Duration
    CreatedAt   time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    Error       string
    Result      *TaskResult
    Metadata    map[string]string
}

// TaskResult contains the outcome of a task execution
type TaskResult struct {
    TaskID     TaskID
    Success    bool
    Output     interface{}
    Error      string
    Duration   time.Duration
    Metrics    TaskMetrics
}

// Worker represents a task processing worker
type Worker struct {
    ID         WorkerID
    State      WorkerState
    CurrentTask *TaskID
    StartedAt  time.Time
    TasksCompleted int
    LastActive time.Time
}

// WorkerPool manages a pool of workers
type WorkerPool struct {
    Size        int
    MinSize     int
    MaxSize     int
    Workers     map[WorkerID]*Worker
    Queue       *PriorityQueue
    Metrics     PoolMetrics
}

// TaskPayload contains type-specific task data
type TaskPayload struct {
    // Container tasks
    Container *ContainerTaskPayload

    // Network tasks
    Network *NetworkTaskPayload

    // Security tasks
    Security *SecurityTaskPayload

    // Health tasks
    Health *HealthTaskPayload
}

// ContainerTaskPayload for container operations
type ContainerTaskPayload struct {
    Operation   ContainerOperation
    ContainerID string
    Spec        *ContainerSpec
    Timeout     time.Duration
}

// NetworkTaskPayload for network operations
type NetworkTaskPayload struct {
    Operation   NetworkOperation
    ContainerID string
    NetworkID   string
    IPAddress   string
    Config      *NetworkConfig
}

// SecurityTaskPayload for security operations
type SecurityTaskPayload struct {
    Operation SecurityOperation
    PolicyID  string
    Policy    *SecurityPolicy
}
```

### Value Objects

```go
// TaskID uniquely identifies a task
type TaskID string

func NewTaskID() TaskID {
    return TaskID(uuid.New().String())
}

// TaskType defines the category of task
type TaskType string

const (
    TaskTypeContainerCreate   TaskType = "container.create"
    TaskTypeContainerStart    TaskType = "container.start"
    TaskTypeContainerStop     TaskType = "container.stop"
    TaskTypeContainerRemove   TaskType = "container.remove"
    TaskTypeNetworkConnect    TaskType = "network.connect"
    TaskTypeNetworkDisconnect TaskType = "network.disconnect"
    TaskTypeSecurityApply     TaskType = "security.apply"
    TaskTypeSecurityRemove    TaskType = "security.remove"
    TaskTypeHealthCheck       TaskType = "health.check"
)

// TaskPriority defines task priority levels
type TaskPriority int

const (
    PriorityLow      TaskPriority = 0
    PriorityNormal   TaskPriority = 50
    PriorityHigh     TaskPriority = 75
    PriorityCritical TaskPriority = 100
)

// TaskState represents the current state of a task
type TaskState string

const (
    TaskStatePending   TaskState = "pending"
    TaskStateQueued    TaskState = "queued"
    TaskStateRunning   TaskState = "running"
    TaskStateCompleted TaskState = "completed"
    TaskStateFailed    TaskState = "failed"
    TaskStateCancelled TaskState = "cancelled"
    TaskStateRetrying  TaskState = "retrying"
)

// WorkerID identifies a worker
type WorkerID string

// WorkerState represents worker status
type WorkerState string

const (
    WorkerStateIdle    WorkerState = "idle"
    WorkerStateBusy    WorkerState = "busy"
    WorkerStateStopped WorkerState = "stopped"
)

// ContainerOperation types
type ContainerOperation string

const (
    ContainerOpCreate  ContainerOperation = "create"
    ContainerOpStart   ContainerOperation = "start"
    ContainerOpStop    ContainerOperation = "stop"
    ContainerOpRemove  ContainerOperation = "remove"
    ContainerOpRestart ContainerOperation = "restart"
)

// NetworkOperation types
type NetworkOperation string

const (
    NetworkOpConnect    NetworkOperation = "connect"
    NetworkOpDisconnect NetworkOperation = "disconnect"
    NetworkOpConfigure  NetworkOperation = "configure"
)

// SecurityOperation types
type SecurityOperation string

const (
    SecurityOpApply  SecurityOperation = "apply"
    SecurityOpRemove SecurityOperation = "remove"
    SecurityOpSync   SecurityOperation = "sync"
)
```

### Domain Logic

```go
// Task state machine
func (t *Task) CanTransitionTo(newState TaskState) bool {
    validTransitions := map[TaskState][]TaskState{
        TaskStatePending:   {TaskStateQueued, TaskStateCancelled},
        TaskStateQueued:    {TaskStateRunning, TaskStateCancelled},
        TaskStateRunning:   {TaskStateCompleted, TaskStateFailed, TaskStateCancelled},
        TaskStateFailed:    {TaskStateRetrying, TaskStateCancelled},
        TaskStateRetrying:  {TaskStateQueued},
        TaskStateCompleted: {}, // Terminal
        TaskStateCancelled: {}, // Terminal
    }

    allowed, exists := validTransitions[t.State]
    if !exists {
        return false
    }

    for _, s := range allowed {
        if s == newState {
            return true
        }
    }
    return false
}

// Transition task to new state
func (t *Task) TransitionTo(newState TaskState) error {
    if !t.CanTransitionTo(newState) {
        return fmt.Errorf("invalid transition from %s to %s", t.State, newState)
    }

    now := time.Now()
    switch newState {
    case TaskStateRunning:
        t.StartedAt = &now
    case TaskStateCompleted, TaskStateFailed, TaskStateCancelled:
        t.CompletedAt = &now
    case TaskStateRetrying:
        t.RetryCount++
    }

    t.State = newState
    return nil
}

// Check if task should retry
func (t *Task) ShouldRetry() bool {
    return t.State == TaskStateFailed &&
           t.RetryCount < t.MaxRetries &&
           t.isRetryableError()
}

func (t *Task) isRetryableError() bool {
    // Network errors, timeouts are retryable
    if strings.Contains(t.Error, "connection refused") ||
       strings.Contains(t.Error, "timeout") ||
       strings.Contains(t.Error, "temporary") {
        return true
    }
    return false
}

// Calculate retry delay with exponential backoff
func (t *Task) RetryDelay() time.Duration {
    base := 1 * time.Second
    maxDelay := 30 * time.Second

    delay := base * time.Duration(1<<uint(t.RetryCount))
    if delay > maxDelay {
        delay = maxDelay
    }

    // Add jitter
    jitter := time.Duration(rand.Int63n(int64(delay / 4)))
    return delay + jitter
}

// Priority queue implementation
type PriorityQueue struct {
    items []*Task
    mu    sync.Mutex
}

func (pq *PriorityQueue) Push(task *Task) {
    pq.mu.Lock()
    defer pq.mu.Unlock()

    pq.items = append(pq.items, task)
    pq.heapifyUp(len(pq.items) - 1)
}

func (pq *PriorityQueue) Pop() *Task {
    pq.mu.Lock()
    defer pq.mu.Unlock()

    if len(pq.items) == 0 {
        return nil
    }

    item := pq.items[0]
    last := len(pq.items) - 1
    pq.items[0] = pq.items[last]
    pq.items = pq.items[:last]

    if len(pq.items) > 0 {
        pq.heapifyDown(0)
    }

    return item
}

func (pq *PriorityQueue) heapifyUp(i int) {
    for i > 0 {
        parent := (i - 1) / 2
        if pq.items[parent].Priority >= pq.items[i].Priority {
            break
        }
        pq.items[parent], pq.items[i] = pq.items[i], pq.items[parent]
        i = parent
    }
}
```

## Inbound Ports

### TaskExecutorService

```go
// TaskExecutorService is the main interface for task operations
type TaskExecutorService interface {
    // Task submission
    SubmitTask(ctx context.Context, task *Task) (*TaskResult, error)
    SubmitTaskAsync(ctx context.Context, task *Task) (TaskID, error)

    // Task management
    CancelTask(ctx context.Context, taskID TaskID) error
    GetTaskStatus(ctx context.Context, taskID TaskID) (*TaskStatus, error)
    GetTaskResult(ctx context.Context, taskID TaskID) (*TaskResult, error)
    ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error)

    // Events
    SubscribeTaskEvents(ctx context.Context) (<-chan TaskEvent, error)
}

// TaskStatus represents current task status
type TaskStatus struct {
    TaskID     TaskID
    State      TaskState
    Progress   float64
    Message    string
    StartedAt  *time.Time
    Duration   *time.Duration
}

// TaskFilter for querying tasks
type TaskFilter struct {
    States   []TaskState
    Types    []TaskType
    Since    time.Time
    Until    time.Time
    Limit    int
}

// TaskEvent represents a task lifecycle event
type TaskEvent struct {
    TaskID    TaskID
    EventType TaskEventType
    Timestamp time.Time
    Data      interface{}
}

// TaskEventType defines types of task events
type TaskEventType string

const (
    TaskEventQueued    TaskEventType = "queued"
    TaskEventStarted   TaskEventType = "started"
    TaskEventProgress  TaskEventType = "progress"
    TaskEventCompleted TaskEventType = "completed"
    TaskEventFailed    TaskEventType = "failed"
    TaskEventRetrying  TaskEventType = "retrying"
    TaskEventCancelled TaskEventType = "cancelled"
)
```

### WorkerPoolService

```go
// WorkerPoolService manages the worker pool
type WorkerPoolService interface {
    // Pool management
    GetPoolStats(ctx context.Context) (*PoolStats, error)
    ScalePool(ctx context.Context, size int) error
    DrainPool(ctx context.Context) error

    // Worker management
    GetWorkerStatus(ctx context.Context, workerID WorkerID) (*WorkerStatus, error)
    ListWorkers(ctx context.Context) ([]*WorkerStatus, error)
}

// PoolStats contains worker pool statistics
type PoolStats struct {
    TotalWorkers   int
    BusyWorkers    int
    IdleWorkers    int
    QueuedTasks    int
    CompletedTasks int64
    FailedTasks    int64
    AvgTaskDuration time.Duration
}

// WorkerStatus represents a worker's current status
type WorkerStatus struct {
    WorkerID      WorkerID
    State         WorkerState
    CurrentTask   *TaskID
    TasksCompleted int
    LastActive    time.Time
}
```

## Outbound Ports

### Component Services

```go
// ContainerService interface to Container Runtime
type ContainerService interface {
    CreateContainer(ctx context.Context, spec *ContainerSpec) (*ContainerInfo, error)
    StartContainer(ctx context.Context, id string) error
    StopContainer(ctx context.Context, id string, timeout time.Duration) error
    RemoveContainer(ctx context.Context, id string, force bool) error
    GetContainer(ctx context.Context, id string) (*ContainerInfo, error)
}

// NetworkService interface to Network Node
type NetworkService interface {
    ConnectContainer(ctx context.Context, containerID, networkID, ip string) error
    DisconnectContainer(ctx context.Context, containerID, networkID string) error
    GetContainerNetwork(ctx context.Context, containerID string) (*ContainerNetworkInfo, error)
}

// SecurityService interface to Security Executor
type SecurityService interface {
    ApplyPolicy(ctx context.Context, policy *SecurityPolicy) error
    RemovePolicy(ctx context.Context, policyID string) error
    GetPolicyStatus(ctx context.Context, policyID string) (*PolicyStatus, error)
}
```

### Infrastructure Ports

```go
// TaskStore persists tasks for durability
type TaskStore interface {
    Save(ctx context.Context, task *Task) error
    Get(ctx context.Context, taskID TaskID) (*Task, error)
    Update(ctx context.Context, task *Task) error
    Delete(ctx context.Context, taskID TaskID) error
    List(ctx context.Context, filter TaskFilter) ([]*Task, error)
    GetPending(ctx context.Context) ([]*Task, error)
}

// ResultReporter reports results back to Engine
type ResultReporter interface {
    ReportResult(ctx context.Context, result *TaskResult) error
    ReportProgress(ctx context.Context, taskID TaskID, progress float64, message string) error
    ReportError(ctx context.Context, taskID TaskID, err error) error
}

// EventEmitter emits task events
type EventEmitter interface {
    Emit(event TaskEvent)
    Subscribe() <-chan TaskEvent
    Unsubscribe(ch <-chan TaskEvent)
}
```

## Use Cases

### TaskDispatchUseCase

```go
type TaskDispatchUseCase struct {
    queue      *PriorityQueue
    store      TaskStore
    reporter   ResultReporter
    events     EventEmitter
    container  ContainerService
    network    NetworkService
    security   SecurityService
}

func (uc *TaskDispatchUseCase) SubmitTask(ctx context.Context, task *Task) (*TaskResult, error) {
    // 1. Validate task
    if err := task.Validate(); err != nil {
        return nil, fmt.Errorf("invalid task: %w", err)
    }

    // 2. Persist task
    task.State = TaskStatePending
    task.CreatedAt = time.Now()
    if err := uc.store.Save(ctx, task); err != nil {
        return nil, fmt.Errorf("failed to save task: %w", err)
    }

    // 3. Queue task
    if err := task.TransitionTo(TaskStateQueued); err != nil {
        return nil, err
    }
    uc.queue.Push(task)
    uc.store.Update(ctx, task)

    uc.events.Emit(TaskEvent{
        TaskID:    task.ID,
        EventType: TaskEventQueued,
        Timestamp: time.Now(),
    })

    // 4. Wait for completion (synchronous mode)
    return uc.waitForCompletion(ctx, task.ID)
}

func (uc *TaskDispatchUseCase) SubmitTaskAsync(ctx context.Context, task *Task) (TaskID, error) {
    // 1. Validate task
    if err := task.Validate(); err != nil {
        return "", fmt.Errorf("invalid task: %w", err)
    }

    // 2. Persist task
    task.State = TaskStatePending
    task.CreatedAt = time.Now()
    if err := uc.store.Save(ctx, task); err != nil {
        return "", fmt.Errorf("failed to save task: %w", err)
    }

    // 3. Queue task
    if err := task.TransitionTo(TaskStateQueued); err != nil {
        return "", err
    }
    uc.queue.Push(task)
    uc.store.Update(ctx, task)

    uc.events.Emit(TaskEvent{
        TaskID:    task.ID,
        EventType: TaskEventQueued,
        Timestamp: time.Now(),
    })

    return task.ID, nil
}

func (uc *TaskDispatchUseCase) CancelTask(ctx context.Context, taskID TaskID) error {
    task, err := uc.store.Get(ctx, taskID)
    if err != nil {
        return fmt.Errorf("task not found: %w", err)
    }

    if !task.CanTransitionTo(TaskStateCancelled) {
        return ErrCannotCancelTask
    }

    if err := task.TransitionTo(TaskStateCancelled); err != nil {
        return err
    }

    if err := uc.store.Update(ctx, task); err != nil {
        return err
    }

    uc.events.Emit(TaskEvent{
        TaskID:    taskID,
        EventType: TaskEventCancelled,
        Timestamp: time.Now(),
    })

    return nil
}

func (uc *TaskDispatchUseCase) waitForCompletion(ctx context.Context, taskID TaskID) (*TaskResult, error) {
    events := uc.events.Subscribe()
    defer uc.events.Unsubscribe(events)

    for {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case event := <-events:
            if event.TaskID != taskID {
                continue
            }

            switch event.EventType {
            case TaskEventCompleted:
                task, _ := uc.store.Get(ctx, taskID)
                return task.Result, nil
            case TaskEventFailed:
                task, _ := uc.store.Get(ctx, taskID)
                return nil, fmt.Errorf("task failed: %s", task.Error)
            case TaskEventCancelled:
                return nil, ErrTaskCancelled
            }
        }
    }
}
```

### TaskRouterUseCase

```go
type TaskRouterUseCase struct {
    container ContainerService
    network   NetworkService
    security  SecurityService
    store     TaskStore
    reporter  ResultReporter
    events    EventEmitter
}

func (uc *TaskRouterUseCase) ExecuteTask(ctx context.Context, task *Task) (*TaskResult, error) {
    // 1. Transition to running
    if err := task.TransitionTo(TaskStateRunning); err != nil {
        return nil, err
    }
    uc.store.Update(ctx, task)

    uc.events.Emit(TaskEvent{
        TaskID:    task.ID,
        EventType: TaskEventStarted,
        Timestamp: time.Now(),
    })

    // 2. Create timeout context
    taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
    defer cancel()

    // 3. Route to appropriate handler
    startTime := time.Now()
    var result *TaskResult
    var err error

    switch {
    case strings.HasPrefix(string(task.Type), "container."):
        result, err = uc.executeContainerTask(taskCtx, task)
    case strings.HasPrefix(string(task.Type), "network."):
        result, err = uc.executeNetworkTask(taskCtx, task)
    case strings.HasPrefix(string(task.Type), "security."):
        result, err = uc.executeSecurityTask(taskCtx, task)
    default:
        err = fmt.Errorf("unknown task type: %s", task.Type)
    }

    duration := time.Since(startTime)

    // 4. Handle result
    if err != nil {
        task.Error = err.Error()
        if task.ShouldRetry() {
            task.TransitionTo(TaskStateRetrying)
            uc.store.Update(ctx, task)
            uc.events.Emit(TaskEvent{
                TaskID:    task.ID,
                EventType: TaskEventRetrying,
                Timestamp: time.Now(),
                Data:      map[string]interface{}{"retry_count": task.RetryCount},
            })
            return nil, err
        }

        task.TransitionTo(TaskStateFailed)
        uc.store.Update(ctx, task)
        uc.reporter.ReportError(ctx, task.ID, err)
        uc.events.Emit(TaskEvent{
            TaskID:    task.ID,
            EventType: TaskEventFailed,
            Timestamp: time.Now(),
            Data:      map[string]interface{}{"error": err.Error()},
        })
        return nil, err
    }

    // 5. Complete successfully
    result.TaskID = task.ID
    result.Success = true
    result.Duration = duration
    task.Result = result
    task.TransitionTo(TaskStateCompleted)
    uc.store.Update(ctx, task)

    uc.reporter.ReportResult(ctx, result)
    uc.events.Emit(TaskEvent{
        TaskID:    task.ID,
        EventType: TaskEventCompleted,
        Timestamp: time.Now(),
        Data:      map[string]interface{}{"duration": duration},
    })

    return result, nil
}

func (uc *TaskRouterUseCase) executeContainerTask(ctx context.Context, task *Task) (*TaskResult, error) {
    payload := task.Payload.Container
    if payload == nil {
        return nil, ErrInvalidPayload
    }

    switch payload.Operation {
    case ContainerOpCreate:
        info, err := uc.container.CreateContainer(ctx, payload.Spec)
        if err != nil {
            return nil, err
        }
        return &TaskResult{Output: info}, nil

    case ContainerOpStart:
        if err := uc.container.StartContainer(ctx, payload.ContainerID); err != nil {
            return nil, err
        }
        return &TaskResult{Output: "started"}, nil

    case ContainerOpStop:
        if err := uc.container.StopContainer(ctx, payload.ContainerID, payload.Timeout); err != nil {
            return nil, err
        }
        return &TaskResult{Output: "stopped"}, nil

    case ContainerOpRemove:
        if err := uc.container.RemoveContainer(ctx, payload.ContainerID, true); err != nil {
            return nil, err
        }
        return &TaskResult{Output: "removed"}, nil

    default:
        return nil, fmt.Errorf("unknown container operation: %s", payload.Operation)
    }
}

func (uc *TaskRouterUseCase) executeNetworkTask(ctx context.Context, task *Task) (*TaskResult, error) {
    payload := task.Payload.Network
    if payload == nil {
        return nil, ErrInvalidPayload
    }

    switch payload.Operation {
    case NetworkOpConnect:
        if err := uc.network.ConnectContainer(ctx, payload.ContainerID, payload.NetworkID, payload.IPAddress); err != nil {
            return nil, err
        }
        return &TaskResult{Output: "connected"}, nil

    case NetworkOpDisconnect:
        if err := uc.network.DisconnectContainer(ctx, payload.ContainerID, payload.NetworkID); err != nil {
            return nil, err
        }
        return &TaskResult{Output: "disconnected"}, nil

    default:
        return nil, fmt.Errorf("unknown network operation: %s", payload.Operation)
    }
}

func (uc *TaskRouterUseCase) executeSecurityTask(ctx context.Context, task *Task) (*TaskResult, error) {
    payload := task.Payload.Security
    if payload == nil {
        return nil, ErrInvalidPayload
    }

    switch payload.Operation {
    case SecurityOpApply:
        if err := uc.security.ApplyPolicy(ctx, payload.Policy); err != nil {
            return nil, err
        }
        return &TaskResult{Output: "applied"}, nil

    case SecurityOpRemove:
        if err := uc.security.RemovePolicy(ctx, payload.PolicyID); err != nil {
            return nil, err
        }
        return &TaskResult{Output: "removed"}, nil

    default:
        return nil, fmt.Errorf("unknown security operation: %s", payload.Operation)
    }
}
```

### WorkerManagementUseCase

```go
type WorkerManagementUseCase struct {
    pool    *WorkerPool
    queue   *PriorityQueue
    router  *TaskRouterUseCase
    store   TaskStore
}

func (uc *WorkerManagementUseCase) Start(ctx context.Context) {
    // Start initial workers
    for i := 0; i < uc.pool.Size; i++ {
        worker := uc.createWorker()
        go uc.runWorker(ctx, worker)
    }

    // Start queue processor
    go uc.processQueue(ctx)
}

func (uc *WorkerManagementUseCase) createWorker() *Worker {
    worker := &Worker{
        ID:        WorkerID(uuid.New().String()),
        State:     WorkerStateIdle,
        StartedAt: time.Now(),
    }
    uc.pool.Workers[worker.ID] = worker
    return worker
}

func (uc *WorkerManagementUseCase) runWorker(ctx context.Context, worker *Worker) {
    for {
        select {
        case <-ctx.Done():
            worker.State = WorkerStateStopped
            return
        default:
            task := uc.queue.Pop()
            if task == nil {
                time.Sleep(100 * time.Millisecond)
                continue
            }

            worker.State = WorkerStateBusy
            worker.CurrentTask = &task.ID
            worker.LastActive = time.Now()

            _, err := uc.router.ExecuteTask(ctx, task)
            if err != nil && task.ShouldRetry() {
                // Re-queue with delay
                go func(t *Task) {
                    time.Sleep(t.RetryDelay())
                    t.TransitionTo(TaskStateQueued)
                    uc.queue.Push(t)
                }(task)
            }

            worker.State = WorkerStateIdle
            worker.CurrentTask = nil
            worker.TasksCompleted++
        }
    }
}

func (uc *WorkerManagementUseCase) ScalePool(ctx context.Context, size int) error {
    if size < uc.pool.MinSize || size > uc.pool.MaxSize {
        return ErrInvalidPoolSize
    }

    currentSize := len(uc.pool.Workers)

    if size > currentSize {
        // Scale up
        for i := 0; i < size-currentSize; i++ {
            worker := uc.createWorker()
            go uc.runWorker(ctx, worker)
        }
    } else if size < currentSize {
        // Scale down (gracefully stop excess workers)
        count := 0
        for id, worker := range uc.pool.Workers {
            if count >= currentSize-size {
                break
            }
            if worker.State == WorkerStateIdle {
                worker.State = WorkerStateStopped
                delete(uc.pool.Workers, id)
                count++
            }
        }
    }

    uc.pool.Size = size
    return nil
}

func (uc *WorkerManagementUseCase) DrainPool(ctx context.Context) error {
    // Stop accepting new tasks
    // Wait for all current tasks to complete
    timeout := time.After(5 * time.Minute)

    for {
        select {
        case <-timeout:
            return ErrDrainTimeout
        default:
            allIdle := true
            for _, worker := range uc.pool.Workers {
                if worker.State == WorkerStateBusy {
                    allIdle = false
                    break
                }
            }
            if allIdle && uc.queue.Len() == 0 {
                return nil
            }
            time.Sleep(1 * time.Second)
        }
    }
}

func (uc *WorkerManagementUseCase) GetPoolStats(ctx context.Context) (*PoolStats, error) {
    stats := &PoolStats{
        TotalWorkers: len(uc.pool.Workers),
        QueuedTasks:  uc.queue.Len(),
    }

    for _, worker := range uc.pool.Workers {
        switch worker.State {
        case WorkerStateIdle:
            stats.IdleWorkers++
        case WorkerStateBusy:
            stats.BusyWorkers++
        }
        stats.CompletedTasks += int64(worker.TasksCompleted)
    }

    return stats, nil
}
```

## Task Execution Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       Task Execution Flow                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Engine                                                                  │
│    │                                                                     │
│    │ 1. SendTask(task)                                                  │
│    ▼                                                                     │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                     Task Executor                                │    │
│  │                                                                  │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Receive    │ 2. Receive via gRPC stream                    │    │
│  │  │    Task      │                                               │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Validate   │ 3. Validate task payload                      │    │
│  │  │   & Queue    │    - Add to priority queue                    │    │
│  │  └──────┬───────┘    - Persist to store                         │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Worker     │ 4. Worker picks up task                       │    │
│  │  │   Dispatch   │                                               │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │    Route     │ 5. Route to component                         │    │
│  │  │  to Handler  │    - Container Runtime                        │    │
│  │  └──────┬───────┘    - Network Node                             │    │
│  │         │            - Security Executor                         │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Execute    │ 6. Execute task                               │    │
│  │  │    Task      │                                               │    │
│  │  └──────┬───────┘                                               │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Report     │ 7. Report result to Engine                    │    │
│  │  │   Result     │                                               │    │
│  │  └──────────────┘                                               │    │
│  │                                                                  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Error Handling

```go
// Domain errors
var (
    ErrTaskNotFound     = errors.New("task not found")
    ErrInvalidPayload   = errors.New("invalid task payload")
    ErrCannotCancelTask = errors.New("task cannot be cancelled in current state")
    ErrTaskCancelled    = errors.New("task was cancelled")
    ErrTaskTimeout      = errors.New("task execution timeout")
    ErrInvalidPoolSize  = errors.New("invalid pool size")
    ErrDrainTimeout     = errors.New("pool drain timeout")
    ErrQueueFull        = errors.New("task queue is full")
)

// Retry policy
type RetryPolicy struct {
    MaxRetries     int
    InitialDelay   time.Duration
    MaxDelay       time.Duration
    BackoffFactor  float64
    RetryableErrors []string
}

func DefaultRetryPolicy() *RetryPolicy {
    return &RetryPolicy{
        MaxRetries:    3,
        InitialDelay:  1 * time.Second,
        MaxDelay:      30 * time.Second,
        BackoffFactor: 2.0,
        RetryableErrors: []string{
            "connection refused",
            "timeout",
            "temporarily unavailable",
        },
    }
}
```

## Testing Strategy

```go
// Unit test for task dispatch
func TestTaskDispatchUseCase_Submit(t *testing.T) {
    mockStore := &MockTaskStore{}
    mockReporter := &MockResultReporter{}
    mockEvents := &MockEventEmitter{}
    queue := NewPriorityQueue()

    uc := &TaskDispatchUseCase{
        queue:    queue,
        store:    mockStore,
        reporter: mockReporter,
        events:   mockEvents,
    }

    task := &Task{
        ID:       NewTaskID(),
        Type:     TaskTypeContainerCreate,
        Priority: PriorityNormal,
        Timeout:  30 * time.Second,
        Payload: TaskPayload{
            Container: &ContainerTaskPayload{
                Operation: ContainerOpCreate,
                Spec:      &ContainerSpec{Name: "test"},
            },
        },
    }

    mockStore.On("Save", mock.Anything, mock.Anything).Return(nil)
    mockStore.On("Update", mock.Anything, mock.Anything).Return(nil)
    mockEvents.On("Emit", mock.Anything).Return()

    // Submit async
    taskID, err := uc.SubmitTaskAsync(context.Background(), task)

    assert.NoError(t, err)
    assert.NotEmpty(t, taskID)
    assert.Equal(t, 1, queue.Len())
    mockStore.AssertExpectations(t)
}

// Integration test for worker pool
func TestWorkerPool_Integration(t *testing.T) {
    mockContainer := &MockContainerService{}
    mockNetwork := &MockNetworkService{}
    mockSecurity := &MockSecurityService{}

    router := &TaskRouterUseCase{
        container: mockContainer,
        network:   mockNetwork,
        security:  mockSecurity,
    }

    pool := &WorkerPool{
        Size:    2,
        MinSize: 1,
        MaxSize: 10,
        Workers: make(map[WorkerID]*Worker),
        Queue:   NewPriorityQueue(),
    }

    uc := &WorkerManagementUseCase{
        pool:   pool,
        queue:  pool.Queue,
        router: router,
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Start pool
    uc.Start(ctx)

    // Get stats
    stats, err := uc.GetPoolStats(ctx)
    require.NoError(t, err)
    assert.Equal(t, 2, stats.TotalWorkers)
}
```

## Related Documents

- [Container Runtime](./container-runtime.md) - Executes container tasks
- [Network Node](./network-node.md) - Executes network tasks
- [Security Executor](./security-executor.md) - Executes security tasks
- [Orchestrator](../engine/orchestrator.md) - Dispatches tasks to Agent
