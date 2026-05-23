package nfs

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/yashg493/cloudbridge/internal/models"
	"github.com/yashg493/cloudbridge/internal/store"
)

// OpType identifies an NFS v3/v4 procedure being simulated.
type OpType string

const (
	OpRead    OpType = "READ"
	OpWrite   OpType = "WRITE"
	OpCreate  OpType = "CREATE"
	OpDelete  OpType = "DELETE"
	OpLookup  OpType = "LOOKUP"
	OpGetAttr OpType = "GETATTR"
	OpSetAttr OpType = "SETATTR"
)

// Operation describes a single simulated NFS call.
type Operation struct {
	Type      OpType
	Namespace string // namespace name or mount path
	Path      string // file path relative to namespace root
	SizeBytes int64  // relevant for WRITE / READ
	Offset    int64  // byte offset for READ / WRITE
}

// Result is the outcome of a simulated NFS operation.
type Result struct {
	Success   bool
	BytesOps  int64  // bytes read or written
	FileID    string // set on CREATE / LOOKUP success
	Error     error
}

// Simulator dispatches simulated NFS operations against the metadata and file store.
// It does NOT perform real I/O; it updates metadata and triggers tiering recalls
// as needed, making it useful for load simulation and integration testing.
type Simulator struct {
	fileRepo *store.FileRepo
	nsRepo   *store.NamespaceRepo
	logger   *zap.Logger
}

// NewSimulator creates a Simulator backed by the given repositories.
func NewSimulator(
	fileRepo *store.FileRepo,
	nsRepo *store.NamespaceRepo,
	logger *zap.Logger,
) *Simulator {
	return &Simulator{fileRepo: fileRepo, nsRepo: nsRepo, logger: logger}
}

// Execute dispatches op to the appropriate handler method.
// Emits nfs_operations_total and nfs_latency_seconds metrics on return.
func (s *Simulator) Execute(ctx context.Context, op Operation) Result {
	// TODO: start timer for nfs_latency_seconds histogram
	s.logger.Debug("nfs op",
		zap.String("type", string(op.Type)),
		zap.String("namespace", op.Namespace),
		zap.String("path", op.Path),
	)

	var result Result
	switch op.Type {
	case OpRead:
		result = s.read(ctx, op)
	case OpWrite:
		result = s.write(ctx, op)
	case OpCreate:
		result = s.create(ctx, op)
	case OpDelete:
		result = s.delete(ctx, op)
	case OpLookup:
		result = s.lookup(ctx, op)
	case OpGetAttr, OpSetAttr:
		// TODO: implement attribute operations
		result = Result{Error: fmt.Errorf("nfs: %s not implemented", op.Type)}
	default:
		result = Result{Error: fmt.Errorf("nfs: unknown op type %q", op.Type)}
	}

	// TODO: record nfs_operations_total{op_type, status} metric
	// TODO: record nfs_latency_seconds histogram observation
	return result
}

// read simulates an NFS READ RPC.
func (s *Simulator) read(ctx context.Context, op Operation) Result {
	// TODO: resolve file by namespace + path via s.fileRepo
	// TODO: if file.Tier != hot, submit a tier-down recall job and return a
	//       synthetic "recall in progress" error (NFS3ERR_JUKEBOX equivalent)
	// TODO: fire-and-forget s.fileRepo.TouchAccessedAt to update heat tracking
	// TODO: return simulated bytes (min(op.SizeBytes, file.SizeBytes - op.Offset))
	_ = models.TierHot // used once tier-check logic is wired in
	return Result{Error: fmt.Errorf("nfs: READ not implemented")}
}

// write simulates an NFS WRITE RPC.
func (s *Simulator) write(ctx context.Context, op Operation) Result {
	// TODO: resolve or create file record
	// TODO: update file.SizeBytes, file.UpdatedAt
	// TODO: return BytesOps = op.SizeBytes
	return Result{Error: fmt.Errorf("nfs: WRITE not implemented")}
}

// create simulates an NFS CREATE RPC.
func (s *Simulator) create(ctx context.Context, op Operation) Result {
	// TODO: resolve namespace, generate UUID, call s.fileRepo.Create
	// TODO: return FileID = new UUID string
	return Result{Error: fmt.Errorf("nfs: CREATE not implemented")}
}

// delete simulates an NFS REMOVE RPC.
func (s *Simulator) delete(ctx context.Context, op Operation) Result {
	// TODO: resolve file, call s.fileRepo.Delete (soft-delete)
	// TODO: if file.Tier != hot, enqueue cloud delete job
	return Result{Error: fmt.Errorf("nfs: DELETE not implemented")}
}

// lookup simulates an NFS LOOKUP RPC.
func (s *Simulator) lookup(ctx context.Context, op Operation) Result {
	// TODO: resolve namespace, query file by path
	// TODO: return FileID if found, else NFS3ERR_NOENT equivalent
	return Result{Error: fmt.Errorf("nfs: LOOKUP not implemented")}
}
