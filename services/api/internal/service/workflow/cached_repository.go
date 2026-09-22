package workflowsvc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	workflowdom "github.com/Paca-AI/api/internal/domain/workflow"
	"github.com/Paca-AI/api/internal/platform/cache"
)

// CachedRepository decorates a workflowdom.Repository with a Redis-backed
// cache for ListStatusRulesByWorkflow — the hot read on every automation
// event (see worker.WorkflowConsumer.applyStatusRule) — invalidated
// whenever a rule is created, updated, or deleted through this same
// decorator. All other methods are delegated directly to the underlying
// repository without caching.
//
// A single CachedRepository instance is shared between workflowsvc.Service
// (HTTP writes) and worker.WorkflowConsumer (automation reads) — see
// bootstrap wiring — which is what makes the invalidation meaningful: a
// rule edited via the API is invisible to a stale cache for the very next
// automation event, not just once some TTL expires.
//
// Cache errors are non-fatal: on a read error the decorator falls through
// to the real repository; on a write/delete error it logs and continues so
// mutations always succeed even when the cache is temporarily unavailable.
type CachedRepository struct {
	repo workflowdom.Repository
	st   *cache.Store
	ttl  time.Duration
	log  *slog.Logger
}

// NewCachedRepository wraps repo with a caching layer backed by st.
// ttl controls how long a cached rule list lives; zero disables caching.
// log receives non-fatal cache warnings.
func NewCachedRepository(repo workflowdom.Repository, st *cache.Store, ttl time.Duration, log *slog.Logger) *CachedRepository {
	return &CachedRepository{repo: repo, st: st, ttl: ttl, log: log}
}

func statusRulesKey(workflowID uuid.UUID) string {
	return fmt.Sprintf("workflow:%s:status-rules", workflowID)
}

// --- Status rules (cached) ---------------------------------------------------

// ListStatusRulesByWorkflow returns workflowID's status rules, reading from
// cache when available and populating it on a miss.
func (c *CachedRepository) ListStatusRulesByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*workflowdom.StatusRule, error) {
	if c.ttl == 0 {
		return c.repo.ListStatusRulesByWorkflow(ctx, workflowID)
	}
	key := statusRulesKey(workflowID)
	var result []*workflowdom.StatusRule
	if ok, err := c.st.Get(ctx, key, &result); ok {
		return result, nil
	} else if err != nil {
		c.log.WarnContext(ctx, "cache: ListStatusRulesByWorkflow get", "err", err)
	}

	result, err := c.repo.ListStatusRulesByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if err := c.st.Set(ctx, key, result, c.ttl); err != nil {
		c.log.WarnContext(ctx, "cache: ListStatusRulesByWorkflow set", "err", err)
	}
	return result, nil
}

// CreateStatusRule delegates to the underlying repository and invalidates
// the cached rule list for sr's workflow.
func (c *CachedRepository) CreateStatusRule(ctx context.Context, sr *workflowdom.StatusRule) error {
	if err := c.repo.CreateStatusRule(ctx, sr); err != nil {
		return err
	}
	c.invalidateStatusRules(ctx, sr.WorkflowID)
	return nil
}

// UpdateStatusRule delegates to the underlying repository and invalidates
// the cached rule list for sr's workflow.
func (c *CachedRepository) UpdateStatusRule(ctx context.Context, sr *workflowdom.StatusRule) error {
	if err := c.repo.UpdateStatusRule(ctx, sr); err != nil {
		return err
	}
	c.invalidateStatusRules(ctx, sr.WorkflowID)
	return nil
}

// DeleteStatusRule looks up the rule first (the Repository interface only
// takes an ID, not the owning workflow) so it knows which workflow's cache
// to invalidate once the delete succeeds.
func (c *CachedRepository) DeleteStatusRule(ctx context.Context, id uuid.UUID) error {
	sr, err := c.repo.FindStatusRuleByID(ctx, id)
	if err != nil {
		return err
	}
	if err := c.repo.DeleteStatusRule(ctx, id); err != nil {
		return err
	}
	c.invalidateStatusRules(ctx, sr.WorkflowID)
	return nil
}

func (c *CachedRepository) invalidateStatusRules(ctx context.Context, workflowID uuid.UUID) {
	if c.ttl == 0 {
		return
	}
	if err := c.st.Delete(ctx, statusRulesKey(workflowID)); err != nil {
		c.log.WarnContext(ctx, "cache: invalidate status rules", "err", err)
	}
}

// FindStatusRuleByID delegates directly to the underlying repository (not cached).
func (c *CachedRepository) FindStatusRuleByID(ctx context.Context, id uuid.UUID) (*workflowdom.StatusRule, error) {
	return c.repo.FindStatusRuleByID(ctx, id)
}

// --- Everything else (pass-through) ------------------------------------------

// CreateWorkflow delegates directly to the underlying repository (not cached).
func (c *CachedRepository) CreateWorkflow(ctx context.Context, w *workflowdom.Workflow) error {
	return c.repo.CreateWorkflow(ctx, w)
}

// FindWorkflowByID delegates directly to the underlying repository (not cached).
func (c *CachedRepository) FindWorkflowByID(ctx context.Context, id uuid.UUID) (*workflowdom.Workflow, error) {
	return c.repo.FindWorkflowByID(ctx, id)
}

// ListWorkflows delegates directly to the underlying repository (not cached).
func (c *CachedRepository) ListWorkflows(ctx context.Context, projectID uuid.UUID, status *workflowdom.Status) ([]*workflowdom.Workflow, error) {
	return c.repo.ListWorkflows(ctx, projectID, status)
}

// UpdateWorkflow delegates directly to the underlying repository (not cached).
func (c *CachedRepository) UpdateWorkflow(ctx context.Context, w *workflowdom.Workflow) error {
	return c.repo.UpdateWorkflow(ctx, w)
}

// DeleteWorkflow delegates directly to the underlying repository (not cached).
func (c *CachedRepository) DeleteWorkflow(ctx context.Context, id uuid.UUID) error {
	return c.repo.DeleteWorkflow(ctx, id)
}

// LoadGraph delegates directly to the underlying repository (not cached).
func (c *CachedRepository) LoadGraph(ctx context.Context, workflowID uuid.UUID) (*workflowdom.Graph, error) {
	return c.repo.LoadGraph(ctx, workflowID)
}

// CreateNode delegates directly to the underlying repository (not cached).
func (c *CachedRepository) CreateNode(ctx context.Context, n *workflowdom.Node) error {
	return c.repo.CreateNode(ctx, n)
}

// FindNodeByID delegates directly to the underlying repository (not cached).
func (c *CachedRepository) FindNodeByID(ctx context.Context, id uuid.UUID) (*workflowdom.Node, error) {
	return c.repo.FindNodeByID(ctx, id)
}

// ListNodesByWorkflow delegates directly to the underlying repository (not cached).
func (c *CachedRepository) ListNodesByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*workflowdom.Node, error) {
	return c.repo.ListNodesByWorkflow(ctx, workflowID)
}

// UpdateNode delegates directly to the underlying repository (not cached).
func (c *CachedRepository) UpdateNode(ctx context.Context, n *workflowdom.Node) error {
	return c.repo.UpdateNode(ctx, n)
}

// DeleteNode delegates directly to the underlying repository (not cached).
func (c *CachedRepository) DeleteNode(ctx context.Context, id uuid.UUID) error {
	return c.repo.DeleteNode(ctx, id)
}

// CreateStatusTransition delegates directly to the underlying repository (not cached).
func (c *CachedRepository) CreateStatusTransition(ctx context.Context, st *workflowdom.StatusTransition) error {
	return c.repo.CreateStatusTransition(ctx, st)
}

// FindStatusTransitionByID delegates directly to the underlying repository (not cached).
func (c *CachedRepository) FindStatusTransitionByID(ctx context.Context, id uuid.UUID) (*workflowdom.StatusTransition, error) {
	return c.repo.FindStatusTransitionByID(ctx, id)
}

// ListStatusTransitionsByWorkflow delegates directly to the underlying repository (not cached).
func (c *CachedRepository) ListStatusTransitionsByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*workflowdom.StatusTransition, error) {
	return c.repo.ListStatusTransitionsByWorkflow(ctx, workflowID)
}

// UpdateStatusTransition delegates directly to the underlying repository (not cached).
func (c *CachedRepository) UpdateStatusTransition(ctx context.Context, st *workflowdom.StatusTransition) error {
	return c.repo.UpdateStatusTransition(ctx, st)
}

// DeleteStatusTransition delegates directly to the underlying repository (not cached).
func (c *CachedRepository) DeleteStatusTransition(ctx context.Context, id uuid.UUID) error {
	return c.repo.DeleteStatusTransition(ctx, id)
}

// CreateEdge delegates directly to the underlying repository (not cached).
func (c *CachedRepository) CreateEdge(ctx context.Context, e *workflowdom.Edge) error {
	return c.repo.CreateEdge(ctx, e)
}

// FindEdgeByID delegates directly to the underlying repository (not cached).
func (c *CachedRepository) FindEdgeByID(ctx context.Context, id uuid.UUID) (*workflowdom.Edge, error) {
	return c.repo.FindEdgeByID(ctx, id)
}

// ListEdgesByWorkflow delegates directly to the underlying repository (not cached).
func (c *CachedRepository) ListEdgesByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*workflowdom.Edge, error) {
	return c.repo.ListEdgesByWorkflow(ctx, workflowID)
}

// DeleteEdge delegates directly to the underlying repository (not cached).
func (c *CachedRepository) DeleteEdge(ctx context.Context, id uuid.UUID) error {
	return c.repo.DeleteEdge(ctx, id)
}

// ListActiveNodesByTaskID delegates directly to the underlying repository (not cached).
func (c *CachedRepository) ListActiveNodesByTaskID(ctx context.Context, taskID uuid.UUID) ([]*workflowdom.Node, error) {
	return c.repo.ListActiveNodesByTaskID(ctx, taskID)
}

// ListIncomingEdges delegates directly to the underlying repository (not cached).
func (c *CachedRepository) ListIncomingEdges(ctx context.Context, targetNodeID uuid.UUID) ([]*workflowdom.Edge, error) {
	return c.repo.ListIncomingEdges(ctx, targetNodeID)
}

// ListWorkflowsByTaskID delegates directly to the underlying repository (not cached).
func (c *CachedRepository) ListWorkflowsByTaskID(ctx context.Context, projectID, taskID uuid.UUID) ([]*workflowdom.Workflow, error) {
	return c.repo.ListWorkflowsByTaskID(ctx, projectID, taskID)
}
