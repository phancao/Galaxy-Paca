// Package workflowsvc_test contains unit tests for the workflow service
// layer. Tests use in-memory fake repositories and do not require any
// infrastructure.
package workflowsvc_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	projectdom "github.com/Paca-AI/api/internal/domain/project"
	taskdom "github.com/Paca-AI/api/internal/domain/task"
	workflowdom "github.com/Paca-AI/api/internal/domain/workflow"
	workflowsvc "github.com/Paca-AI/api/internal/service/workflow"
)

// ---------------------------------------------------------------------------
// Fake repositories
// ---------------------------------------------------------------------------

type fakeWorkflowRepo struct {
	mu          sync.Mutex
	workflows   map[uuid.UUID]*workflowdom.Workflow
	nodes       map[uuid.UUID]*workflowdom.Node
	rules       map[uuid.UUID]*workflowdom.StatusRule
	transitions map[uuid.UUID]*workflowdom.StatusTransition
	edges       map[uuid.UUID]*workflowdom.Edge

	// simulateRuleConflictOnce/simulateTransitionConflictOnce let a test
	// emulate two concurrent SetStatusRule/SetStatusTransition callers
	// racing: the next CreateStatusRule/CreateStatusTransition call inserts
	// a "concurrently created" row for the same status behind the caller's
	// back and returns the conflict error a real unique-constraint violation
	// would produce, instead of actually creating the caller's row.
	simulateRuleConflictOnce       bool
	simulateTransitionConflictOnce bool

	listStatusRulesCalls int // counts real ListStatusRulesByWorkflow calls, to assert CachedRepository hits/invalidations
}

func newFakeWorkflowRepo() *fakeWorkflowRepo {
	return &fakeWorkflowRepo{
		workflows:   make(map[uuid.UUID]*workflowdom.Workflow),
		nodes:       make(map[uuid.UUID]*workflowdom.Node),
		rules:       make(map[uuid.UUID]*workflowdom.StatusRule),
		transitions: make(map[uuid.UUID]*workflowdom.StatusTransition),
		edges:       make(map[uuid.UUID]*workflowdom.Edge),
	}
}

func (r *fakeWorkflowRepo) CreateWorkflow(_ context.Context, w *workflowdom.Workflow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *w
	r.workflows[w.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) FindWorkflowByID(_ context.Context, id uuid.UUID) (*workflowdom.Workflow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workflows[id]
	if !ok {
		return nil, workflowdom.ErrNotFound
	}
	cp := *w
	return &cp, nil
}

func (r *fakeWorkflowRepo) ListWorkflows(_ context.Context, projectID uuid.UUID, status *workflowdom.Status) ([]*workflowdom.Workflow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*workflowdom.Workflow
	for _, w := range r.workflows {
		if w.ProjectID != projectID {
			continue
		}
		if status != nil && w.Status != *status {
			continue
		}
		cp := *w
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeWorkflowRepo) UpdateWorkflow(_ context.Context, w *workflowdom.Workflow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.workflows[w.ID]; !ok {
		return workflowdom.ErrNotFound
	}
	cp := *w
	r.workflows[w.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) DeleteWorkflow(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.workflows[id]; !ok {
		return workflowdom.ErrNotFound
	}
	delete(r.workflows, id)
	return nil
}

func (r *fakeWorkflowRepo) LoadGraph(ctx context.Context, workflowID uuid.UUID) (*workflowdom.Graph, error) {
	w, err := r.FindWorkflowByID(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	nodes, _ := r.ListNodesByWorkflow(ctx, workflowID)
	edges, _ := r.ListEdgesByWorkflow(ctx, workflowID)
	rules, _ := r.ListStatusRulesByWorkflow(ctx, workflowID)
	transitions, _ := r.ListStatusTransitionsByWorkflow(ctx, workflowID)
	return &workflowdom.Graph{
		Workflow:          w,
		Nodes:             nodes,
		StatusRules:       rules,
		StatusTransitions: transitions,
		Edges:             edges,
	}, nil
}

func (r *fakeWorkflowRepo) CreateNode(_ context.Context, n *workflowdom.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.nodes {
		if existing.WorkflowID == n.WorkflowID && existing.TaskID == n.TaskID {
			return workflowdom.ErrNodeDuplicateTask
		}
	}
	cp := *n
	r.nodes[n.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) FindNodeByID(_ context.Context, id uuid.UUID) (*workflowdom.Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.nodes[id]
	if !ok {
		return nil, workflowdom.ErrNodeNotFound
	}
	cp := *n
	return &cp, nil
}

func (r *fakeWorkflowRepo) ListNodesByWorkflow(_ context.Context, workflowID uuid.UUID) ([]*workflowdom.Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*workflowdom.Node
	for _, n := range r.nodes {
		if n.WorkflowID == workflowID {
			cp := *n
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeWorkflowRepo) UpdateNode(_ context.Context, n *workflowdom.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[n.ID]; !ok {
		return workflowdom.ErrNodeNotFound
	}
	cp := *n
	r.nodes[n.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) DeleteNode(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[id]; !ok {
		return workflowdom.ErrNodeNotFound
	}
	delete(r.nodes, id)
	return nil
}

func (r *fakeWorkflowRepo) CreateStatusRule(_ context.Context, sr *workflowdom.StatusRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.simulateRuleConflictOnce {
		r.simulateRuleConflictOnce = false
		concurrent := *sr
		concurrent.ID = uuid.New()
		r.rules[concurrent.ID] = &concurrent
		return workflowdom.ErrStatusRuleConflict
	}
	cp := *sr
	r.rules[sr.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) FindStatusRuleByID(_ context.Context, id uuid.UUID) (*workflowdom.StatusRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sr, ok := r.rules[id]
	if !ok {
		return nil, workflowdom.ErrStatusRuleNotFound
	}
	cp := *sr
	return &cp, nil
}

func (r *fakeWorkflowRepo) ListStatusRulesByWorkflow(_ context.Context, workflowID uuid.UUID) ([]*workflowdom.StatusRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listStatusRulesCalls++
	var out []*workflowdom.StatusRule
	for _, sr := range r.rules {
		if sr.WorkflowID == workflowID {
			cp := *sr
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeWorkflowRepo) UpdateStatusRule(_ context.Context, sr *workflowdom.StatusRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[sr.ID]; !ok {
		return workflowdom.ErrStatusRuleNotFound
	}
	cp := *sr
	r.rules[sr.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) DeleteStatusRule(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[id]; !ok {
		return workflowdom.ErrStatusRuleNotFound
	}
	delete(r.rules, id)
	return nil
}

func (r *fakeWorkflowRepo) CreateStatusTransition(_ context.Context, st *workflowdom.StatusTransition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.simulateTransitionConflictOnce {
		r.simulateTransitionConflictOnce = false
		concurrent := *st
		concurrent.ID = uuid.New()
		r.transitions[concurrent.ID] = &concurrent
		return workflowdom.ErrStatusTransitionConflict
	}
	cp := *st
	r.transitions[st.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) FindStatusTransitionByID(_ context.Context, id uuid.UUID) (*workflowdom.StatusTransition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.transitions[id]
	if !ok {
		return nil, workflowdom.ErrStatusTransitionNotFound
	}
	cp := *st
	return &cp, nil
}

func (r *fakeWorkflowRepo) ListStatusTransitionsByWorkflow(_ context.Context, workflowID uuid.UUID) ([]*workflowdom.StatusTransition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*workflowdom.StatusTransition
	for _, st := range r.transitions {
		if st.WorkflowID == workflowID {
			cp := *st
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeWorkflowRepo) UpdateStatusTransition(_ context.Context, st *workflowdom.StatusTransition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.transitions[st.ID]; !ok {
		return workflowdom.ErrStatusTransitionNotFound
	}
	cp := *st
	r.transitions[st.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) DeleteStatusTransition(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.transitions[id]; !ok {
		return workflowdom.ErrStatusTransitionNotFound
	}
	delete(r.transitions, id)
	return nil
}

func (r *fakeWorkflowRepo) CreateEdge(_ context.Context, e *workflowdom.Edge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.edges {
		if existing.SourceNodeID == e.SourceNodeID && existing.TargetNodeID == e.TargetNodeID {
			return workflowdom.ErrEdgeDuplicate
		}
	}
	cp := *e
	r.edges[e.ID] = &cp
	return nil
}

func (r *fakeWorkflowRepo) FindEdgeByID(_ context.Context, id uuid.UUID) (*workflowdom.Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.edges[id]
	if !ok {
		return nil, workflowdom.ErrEdgeNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *fakeWorkflowRepo) ListEdgesByWorkflow(_ context.Context, workflowID uuid.UUID) ([]*workflowdom.Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*workflowdom.Edge
	for _, e := range r.edges {
		if e.WorkflowID == workflowID {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeWorkflowRepo) DeleteEdge(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.edges[id]; !ok {
		return workflowdom.ErrEdgeNotFound
	}
	delete(r.edges, id)
	return nil
}

func (r *fakeWorkflowRepo) ListActiveNodesByTaskID(_ context.Context, taskID uuid.UUID) ([]*workflowdom.Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*workflowdom.Node
	for _, n := range r.nodes {
		if n.TaskID != taskID {
			continue
		}
		w, ok := r.workflows[n.WorkflowID]
		if !ok || w.Status != workflowdom.StatusActive {
			continue
		}
		cp := *n
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeWorkflowRepo) ListWorkflowsByTaskID(_ context.Context, projectID, taskID uuid.UUID) ([]*workflowdom.Workflow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[uuid.UUID]bool)
	var out []*workflowdom.Workflow
	for _, n := range r.nodes {
		if n.TaskID != taskID || seen[n.WorkflowID] {
			continue
		}
		w, ok := r.workflows[n.WorkflowID]
		if !ok || w.ProjectID != projectID {
			continue
		}
		seen[n.WorkflowID] = true
		cp := *w
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeWorkflowRepo) ListIncomingEdges(_ context.Context, targetNodeID uuid.UUID) ([]*workflowdom.Edge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*workflowdom.Edge
	for _, e := range r.edges {
		if e.TargetNodeID == targetNodeID {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

// fakeTaskLookup is a minimal stand-in for the taskLookup interface.
type fakeTaskLookup struct {
	tasks    map[uuid.UUID]*taskdom.Task
	statuses map[uuid.UUID]*taskdom.TaskStatus
}

func newFakeTaskLookup() *fakeTaskLookup {
	return &fakeTaskLookup{
		tasks:    make(map[uuid.UUID]*taskdom.Task),
		statuses: make(map[uuid.UUID]*taskdom.TaskStatus),
	}
}

func (f *fakeTaskLookup) FindTaskByID(_ context.Context, id uuid.UUID) (*taskdom.Task, error) {
	t, ok := f.tasks[id]
	if !ok {
		return nil, taskdom.ErrTaskNotFound
	}
	return t, nil
}

func (f *fakeTaskLookup) FindTaskStatusByID(_ context.Context, id uuid.UUID) (*taskdom.TaskStatus, error) {
	s, ok := f.statuses[id]
	if !ok {
		return nil, taskdom.ErrStatusNotFound
	}
	return s, nil
}

func (f *fakeTaskLookup) ListTaskStatuses(_ context.Context, projectID uuid.UUID) ([]*taskdom.TaskStatus, error) {
	var out []*taskdom.TaskStatus
	for _, s := range f.statuses {
		if s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out, nil
}

// fakeMemberLookup is a minimal stand-in for the memberLookup interface.
// order records insertion order so ListMembers is deterministic (the real
// repo orders alphabetically by username/handle; tests only need "first
// added" to be stable across runs).
type fakeMemberLookup struct {
	members map[uuid.UUID]*projectdom.ProjectMember
	order   []uuid.UUID
}

func newFakeMemberLookup() *fakeMemberLookup {
	return &fakeMemberLookup{members: make(map[uuid.UUID]*projectdom.ProjectMember)}
}

func (f *fakeMemberLookup) ListMembers(_ context.Context, projectID uuid.UUID) ([]*projectdom.ProjectMember, error) {
	var out []*projectdom.ProjectMember
	for _, id := range f.order {
		if m, ok := f.members[id]; ok && m.ProjectID == projectID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeMemberLookup) FindMemberByID(_ context.Context, memberID uuid.UUID) (*projectdom.ProjectMember, error) {
	m, ok := f.members[memberID]
	if !ok {
		return nil, projectdom.ErrMemberNotFound
	}
	return m, nil
}

func (f *fakeMemberLookup) FindMemberByActor(_ context.Context, projectID, actorID uuid.UUID, agentID *uuid.UUID) (*projectdom.ProjectMember, error) {
	for _, m := range f.members {
		if m.ProjectID != projectID {
			continue
		}
		if agentID != nil {
			if m.AgentID != nil && *m.AgentID == *agentID {
				return m, nil
			}
			continue
		}
		if m.UserID == actorID {
			return m, nil
		}
	}
	return nil, projectdom.ErrMemberNotFound
}

// ---------------------------------------------------------------------------
// Test fixture helpers
// ---------------------------------------------------------------------------

type fixture struct {
	repo       *fakeWorkflowRepo
	taskLookup *fakeTaskLookup
	memberRepo *fakeMemberLookup
	svc        *workflowsvc.Service
	projectID  uuid.UUID
}

func newFixture() *fixture {
	repo := newFakeWorkflowRepo()
	taskLookup := newFakeTaskLookup()
	memberRepo := newFakeMemberLookup()
	return &fixture{
		repo:       repo,
		taskLookup: taskLookup,
		memberRepo: memberRepo,
		svc:        workflowsvc.New(repo, taskLookup, memberRepo, nil),
		projectID:  uuid.New(),
	}
}

func (f *fixture) addTask(projectID uuid.UUID) *taskdom.Task {
	t := &taskdom.Task{ID: uuid.New(), ProjectID: projectID, Title: "task"}
	f.taskLookup.tasks[t.ID] = t
	return t
}

func (f *fixture) addStatus(projectID uuid.UUID, category taskdom.StatusCategory) *taskdom.TaskStatus {
	s := &taskdom.TaskStatus{ID: uuid.New(), ProjectID: projectID, Name: "status", Category: category}
	f.taskLookup.statuses[s.ID] = s
	return s
}

func (f *fixture) addMember(projectID uuid.UUID) *projectdom.ProjectMember {
	m := &projectdom.ProjectMember{ID: uuid.New(), ProjectID: projectID}
	f.memberRepo.members[m.ID] = m
	f.memberRepo.order = append(f.memberRepo.order, m.ID)
	return m
}

func (f *fixture) addAgentMember(projectID uuid.UUID) *projectdom.ProjectMember {
	agentID := uuid.New()
	m := &projectdom.ProjectMember{
		ID: uuid.New(), ProjectID: projectID, MemberType: "agent", AgentID: &agentID,
	}
	f.memberRepo.members[m.ID] = m
	f.memberRepo.order = append(f.memberRepo.order, m.ID)
	return m
}

func mustCreateWorkflow(t *testing.T, f *fixture) *workflowdom.Workflow {
	t.Helper()
	w, err := f.svc.CreateWorkflow(context.Background(), workflowdom.CreateWorkflowInput{
		ProjectID: f.projectID, Name: "wf",
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	return w
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateWorkflow_RejectsEmptyName(t *testing.T) {
	f := newFixture()
	_, err := f.svc.CreateWorkflow(context.Background(), workflowdom.CreateWorkflowInput{ProjectID: f.projectID, Name: "  "})
	if !errors.Is(err, workflowdom.ErrNameInvalid) {
		t.Fatalf("expected ErrNameInvalid, got %v", err)
	}
}

func TestCreateWorkflow_StartsAsDraft(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	if w.Status != workflowdom.StatusDraft {
		t.Fatalf("expected draft status, got %v", w.Status)
	}
}

func TestAddNode_RejectsDuplicateTask(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	task := f.addTask(f.projectID)

	if _, err := f.svc.AddNode(context.Background(), f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: task.ID}); err != nil {
		t.Fatalf("first AddNode: %v", err)
	}
	_, err := f.svc.AddNode(context.Background(), f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: task.ID})
	if !errors.Is(err, workflowdom.ErrNodeDuplicateTask) {
		t.Fatalf("expected ErrNodeDuplicateTask, got %v", err)
	}
}

func TestAddNode_RejectsCrossProjectTask(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	otherProjectTask := f.addTask(uuid.New())

	_, err := f.svc.AddNode(context.Background(), f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: otherProjectTask.ID})
	if !errors.Is(err, workflowdom.ErrNodeTaskCrossProject) {
		t.Fatalf("expected ErrNodeTaskCrossProject, got %v", err)
	}
}

func TestAddNode_AllowedWhenActive_RejectsWhenArchived(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	task := f.addTask(f.projectID)
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: task.ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	done := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: done.ID}); err != nil {
		t.Fatalf("SetStatusTransition: %v", err)
	}
	member := f.addMember(f.projectID)
	if _, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{StatusID: done.ID, AssigneeMemberID: member.ID}); err != nil {
		t.Fatalf("SetStatusRule: %v", err)
	}
	if _, err := f.svc.Activate(ctx, f.projectID, w.ID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	otherTask := f.addTask(f.projectID)
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: otherTask.ID}); err != nil {
		t.Fatalf("expected AddNode to succeed while active, got %v", err)
	}

	if _, err := f.svc.Archive(ctx, f.projectID, w.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	thirdTask := f.addTask(f.projectID)
	_, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: thirdTask.ID})
	if !errors.Is(err, workflowdom.ErrArchived) {
		t.Fatalf("expected ErrArchived, got %v", err)
	}
}

func TestAddEdge_RejectsSelfLoop(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	task := f.addTask(f.projectID)
	node, err := f.svc.AddNode(context.Background(), f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: task.ID})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	_, err = f.svc.AddEdge(context.Background(), f.projectID, w.ID, workflowdom.AddEdgeInput{SourceNodeID: node.ID, TargetNodeID: node.ID})
	if !errors.Is(err, workflowdom.ErrEdgeSelfLoop) {
		t.Fatalf("expected ErrEdgeSelfLoop, got %v", err)
	}
}

func TestAddEdge_RejectsCycle(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()

	a, _ := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID})
	b, _ := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID})
	c, _ := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID})

	if _, err := f.svc.AddEdge(ctx, f.projectID, w.ID, workflowdom.AddEdgeInput{SourceNodeID: a.ID, TargetNodeID: b.ID}); err != nil {
		t.Fatalf("AddEdge a->b: %v", err)
	}
	if _, err := f.svc.AddEdge(ctx, f.projectID, w.ID, workflowdom.AddEdgeInput{SourceNodeID: b.ID, TargetNodeID: c.ID}); err != nil {
		t.Fatalf("AddEdge b->c: %v", err)
	}

	// c -> a would close the loop a -> b -> c -> a.
	_, err := f.svc.AddEdge(ctx, f.projectID, w.ID, workflowdom.AddEdgeInput{SourceNodeID: c.ID, TargetNodeID: a.ID})
	if !errors.Is(err, workflowdom.ErrEdgeCycle) {
		t.Fatalf("expected ErrEdgeCycle, got %v", err)
	}
}

func TestAddEdge_RejectsDuplicate(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	a, _ := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID})
	b, _ := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID})

	if _, err := f.svc.AddEdge(ctx, f.projectID, w.ID, workflowdom.AddEdgeInput{SourceNodeID: a.ID, TargetNodeID: b.ID}); err != nil {
		t.Fatalf("first AddEdge: %v", err)
	}
	_, err := f.svc.AddEdge(ctx, f.projectID, w.ID, workflowdom.AddEdgeInput{SourceNodeID: a.ID, TargetNodeID: b.ID})
	if !errors.Is(err, workflowdom.ErrEdgeDuplicate) {
		t.Fatalf("expected ErrEdgeDuplicate, got %v", err)
	}
}

func TestActivate_RejectsEmptyWorkflow(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	_, err := f.svc.Activate(context.Background(), f.projectID, w.ID)
	if !errors.Is(err, workflowdom.ErrActivateNoNodes) {
		t.Fatalf("expected ErrActivateNoNodes, got %v", err)
	}
}

func TestActivate_RejectsUndeterminedDoneStatus(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()

	// No status-transition entries configured at all (the project had no
	// statuses at workflow-creation time, so nothing was auto-seeded) means
	// there's no derivable done status.
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	_, err := f.svc.Activate(ctx, f.projectID, w.ID)
	if !errors.Is(err, workflowdom.ErrActivateDoneStatusUndetermined) {
		t.Fatalf("expected ErrActivateDoneStatusUndetermined, got %v", err)
	}
}

func TestActivate_RejectsAmbiguousDoneStatus(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()

	// Two terminal (no next-status) entries means the done status can't be
	// derived unambiguously.
	statusA := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	statusB := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: statusA.ID}); err != nil {
		t.Fatalf("SetStatusTransition A: %v", err)
	}
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: statusB.ID}); err != nil {
		t.Fatalf("SetStatusTransition B: %v", err)
	}
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	_, err := f.svc.Activate(ctx, f.projectID, w.ID)
	if !errors.Is(err, workflowdom.ErrActivateDoneStatusUndetermined) {
		t.Fatalf("expected ErrActivateDoneStatusUndetermined, got %v", err)
	}
}

func TestActivate_SucceedsWithDerivedDoneStatus(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()

	ready := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	done := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: ready.ID, NextStatusID: &done.ID}); err != nil {
		t.Fatalf("SetStatusTransition ready->done: %v", err)
	}
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: done.ID}); err != nil {
		t.Fatalf("SetStatusTransition done (terminal): %v", err)
	}
	member := f.addMember(f.projectID)
	if _, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{StatusID: ready.ID, AssigneeMemberID: member.ID}); err != nil {
		t.Fatalf("SetStatusRule: %v", err)
	}

	a, _ := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID})
	b, _ := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID})
	if _, err := f.svc.AddEdge(ctx, f.projectID, w.ID, workflowdom.AddEdgeInput{SourceNodeID: a.ID, TargetNodeID: b.ID}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	activated, err := f.svc.Activate(ctx, f.projectID, w.ID)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if activated.Status != workflowdom.StatusActive {
		t.Fatalf("expected active status, got %v", activated.Status)
	}
}

func TestActivate_RejectsWhenTaskMissingFromProject(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	task := f.addTask(f.projectID)
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: task.ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	// Simulate the task having been deleted after the node was created.
	delete(f.taskLookup.tasks, task.ID)

	_, err := f.svc.Activate(ctx, f.projectID, w.ID)
	if !errors.Is(err, workflowdom.ErrActivateTaskMissing) {
		t.Fatalf("expected ErrActivateTaskMissing, got %v", err)
	}
}

// TestActivate_RejectsNoStatusRules guards against a workflow activating
// with zero status rules: both automation events reassign strictly by
// status-rule lookup, so such a workflow would run forever without ever
// doing anything (see the all-agent-project case in
// TestCreateWorkflow_SkipsDefaultStatusRules_WhenNoHumanMemberExists, where
// seedDefaultStatusRules silently seeds none).
func TestActivate_RejectsNoStatusRules(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	done := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: done.ID}); err != nil {
		t.Fatalf("SetStatusTransition: %v", err)
	}

	_, err := f.svc.Activate(ctx, f.projectID, w.ID)
	if !errors.Is(err, workflowdom.ErrActivateNoStatusRules) {
		t.Fatalf("expected ErrActivateNoStatusRules, got %v", err)
	}
}

func TestSetStatusRule_RejectsCrossProjectMember(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	status := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	otherProjectMember := f.addMember(uuid.New())

	_, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{
		StatusID: status.ID, AssigneeMemberID: otherProjectMember.ID,
	})
	if !errors.Is(err, workflowdom.ErrStatusRuleCrossProject) {
		t.Fatalf("expected ErrStatusRuleCrossProject, got %v", err)
	}
}

func TestSetStatusRule_UpsertsExistingRule(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	status := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	member1 := f.addMember(f.projectID)
	member2 := f.addMember(f.projectID)

	rule1, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{StatusID: status.ID, AssigneeMemberID: member1.ID})
	if err != nil {
		t.Fatalf("SetStatusRule (create): %v", err)
	}
	rule2, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{StatusID: status.ID, AssigneeMemberID: member2.ID})
	if err != nil {
		t.Fatalf("SetStatusRule (update): %v", err)
	}
	if rule1.ID != rule2.ID {
		t.Fatalf("expected upsert to reuse the same rule ID, got %v vs %v", rule1.ID, rule2.ID)
	}
	if rule2.AssigneeMemberID != member2.ID {
		t.Fatalf("expected assignee to be updated to member2")
	}

	rules, err := f.repo.ListStatusRulesByWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListStatusRulesByWorkflow: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected exactly one rule after upsert, got %d", len(rules))
	}
}

func TestSetStatusRule_RetriesOnConcurrentCreateConflict(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	status := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	member := f.addMember(f.projectID)

	// Simulate another request's SetStatusRule call winning the race and
	// creating the row first; our CreateStatusRule call should get the
	// conflict error and retry, finding and updating that row instead of
	// surfacing a raw duplicate-key error to the caller.
	f.repo.simulateRuleConflictOnce = true

	rule, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{
		StatusID: status.ID, AssigneeMemberID: member.ID,
	})
	if err != nil {
		t.Fatalf("SetStatusRule: expected the conflict to be retried transparently, got error: %v", err)
	}
	if rule.AssigneeMemberID != member.ID {
		t.Fatalf("expected the retried update to set the requested assignee, got %v", rule.AssigneeMemberID)
	}

	rules, err := f.repo.ListStatusRulesByWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListStatusRulesByWorkflow: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected exactly one rule after retry, got %d", len(rules))
	}
}

func TestRevertToDraft_ReenablesEditing(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	done := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: done.ID}); err != nil {
		t.Fatalf("SetStatusTransition: %v", err)
	}
	member := f.addMember(f.projectID)
	if _, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{StatusID: done.ID, AssigneeMemberID: member.ID}); err != nil {
		t.Fatalf("SetStatusRule: %v", err)
	}
	if _, err := f.svc.Activate(ctx, f.projectID, w.ID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	reverted, err := f.svc.RevertToDraft(ctx, f.projectID, w.ID)
	if err != nil {
		t.Fatalf("RevertToDraft: %v", err)
	}
	if reverted.Status != workflowdom.StatusDraft {
		t.Fatalf("expected draft status, got %v", reverted.Status)
	}

	// Editing should now succeed again.
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode after revert-to-draft: %v", err)
	}
}

func TestRevertToDraft_RejectsArchived(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	if _, err := f.svc.AddNode(ctx, f.projectID, w.ID, workflowdom.AddNodeInput{TaskID: f.addTask(f.projectID).ID}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	done := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	if _, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: done.ID}); err != nil {
		t.Fatalf("SetStatusTransition: %v", err)
	}
	member := f.addMember(f.projectID)
	if _, err := f.svc.SetStatusRule(ctx, f.projectID, w.ID, workflowdom.SetStatusRuleInput{StatusID: done.ID, AssigneeMemberID: member.ID}); err != nil {
		t.Fatalf("SetStatusRule: %v", err)
	}
	if _, err := f.svc.Activate(ctx, f.projectID, w.ID); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := f.svc.Archive(ctx, f.projectID, w.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	if _, err := f.svc.RevertToDraft(ctx, f.projectID, w.ID); !errors.Is(err, workflowdom.ErrNotActive) {
		t.Fatalf("expected ErrNotActive reverting an archived workflow, got %v", err)
	}
}

func TestSetStatusTransition_UpsertsExistingEntry(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	status := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	next1 := f.addStatus(f.projectID, taskdom.StatusCategoryInProgress)
	next2 := f.addStatus(f.projectID, taskdom.StatusCategoryDone)

	t1, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: status.ID, NextStatusID: &next1.ID})
	if err != nil {
		t.Fatalf("SetStatusTransition (create): %v", err)
	}
	t2, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: status.ID, NextStatusID: &next2.ID})
	if err != nil {
		t.Fatalf("SetStatusTransition (update): %v", err)
	}
	if t1.ID != t2.ID {
		t.Fatalf("expected upsert to reuse the same transition ID, got %v vs %v", t1.ID, t2.ID)
	}
	if t2.NextStatusID == nil || *t2.NextStatusID != next2.ID {
		t.Fatalf("expected next status to be updated to next2, got %+v", t2.NextStatusID)
	}

	transitions, err := f.repo.ListStatusTransitionsByWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListStatusTransitionsByWorkflow: %v", err)
	}
	if len(transitions) != 1 {
		t.Fatalf("expected exactly one transition after upsert, got %d", len(transitions))
	}
}

func TestSetStatusTransition_RetriesOnConcurrentCreateConflict(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	status := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	next := f.addStatus(f.projectID, taskdom.StatusCategoryDone)

	// Simulate another request's SetStatusTransition call winning the race
	// and creating the row first.
	f.repo.simulateTransitionConflictOnce = true

	transition, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{
		StatusID: status.ID, NextStatusID: &next.ID,
	})
	if err != nil {
		t.Fatalf("SetStatusTransition: expected the conflict to be retried transparently, got error: %v", err)
	}
	if transition.NextStatusID == nil || *transition.NextStatusID != next.ID {
		t.Fatalf("expected the retried update to set the requested next status, got %+v", transition.NextStatusID)
	}

	transitions, err := f.repo.ListStatusTransitionsByWorkflow(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListStatusTransitionsByWorkflow: %v", err)
	}
	if len(transitions) != 1 {
		t.Fatalf("expected exactly one transition after retry, got %d", len(transitions))
	}
}

func TestSetStatusTransition_RejectsSelfLoop(t *testing.T) {
	f := newFixture()
	w := mustCreateWorkflow(t, f)
	ctx := context.Background()
	status := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)

	_, err := f.svc.SetStatusTransition(ctx, f.projectID, w.ID, workflowdom.SetStatusTransitionInput{StatusID: status.ID, NextStatusID: &status.ID})
	if !errors.Is(err, workflowdom.ErrStatusTransitionSelfLoop) {
		t.Fatalf("expected ErrStatusTransitionSelfLoop, got %v", err)
	}
}

func TestCreateWorkflow_SeedsDefaultStatusTransitionsByPosition(t *testing.T) {
	f := newFixture()
	// Add statuses out of position order to prove board position (not
	// insertion order) drives the seeded chain.
	inProgress := f.addStatus(f.projectID, taskdom.StatusCategoryInProgress)
	inProgress.Position = 1
	todo := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	todo.Position = 0
	done := f.addStatus(f.projectID, taskdom.StatusCategoryDone)
	done.Position = 2

	w := mustCreateWorkflow(t, f)

	transitions, err := f.repo.ListStatusTransitionsByWorkflow(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("ListStatusTransitionsByWorkflow: %v", err)
	}
	if len(transitions) != 3 {
		t.Fatalf("expected 3 auto-seeded transitions, got %d", len(transitions))
	}
	byStatus := make(map[uuid.UUID]*workflowdom.StatusTransition, len(transitions))
	for _, tr := range transitions {
		byStatus[tr.StatusID] = tr
	}
	if got := byStatus[todo.ID].NextStatusID; got == nil || *got != inProgress.ID {
		t.Fatalf("expected todo's next to be in-progress, got %+v", got)
	}
	if got := byStatus[inProgress.ID].NextStatusID; got == nil || *got != done.ID {
		t.Fatalf("expected in-progress's next to be done, got %+v", got)
	}
	if got := byStatus[done.ID].NextStatusID; got != nil {
		t.Fatalf("expected done to be terminal (nil next), got %v", *got)
	}
	doneID, ok := workflowdom.DeriveDoneStatusID(transitions)
	if !ok || doneID != done.ID {
		t.Fatalf("expected derived done status to be done (%v), got %v (ok=%v)", done.ID, doneID, ok)
	}
}

func TestCreateWorkflow_SeedsDefaultStatusRules_UsesHumanCreator(t *testing.T) {
	f := newFixture()
	userID := uuid.New()
	creator := f.addMember(f.projectID)
	creator.UserID = userID
	s1 := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)
	s2 := f.addStatus(f.projectID, taskdom.StatusCategoryDone)

	w, err := f.svc.CreateWorkflow(context.Background(), workflowdom.CreateWorkflowInput{
		ProjectID: f.projectID, Name: "wf", CreatedBy: &userID,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if w.CreatedBy == nil || *w.CreatedBy != creator.ID {
		t.Fatalf("expected workflow CreatedBy to resolve to the human creator %v, got %+v", creator.ID, w.CreatedBy)
	}

	rules, err := f.repo.ListStatusRulesByWorkflow(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("ListStatusRulesByWorkflow: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 default status rules (one per status), got %d", len(rules))
	}
	byStatus := make(map[uuid.UUID]*workflowdom.StatusRule, len(rules))
	for _, r := range rules {
		byStatus[r.StatusID] = r
	}
	for _, s := range []*taskdom.TaskStatus{s1, s2} {
		r, ok := byStatus[s.ID]
		if !ok {
			t.Fatalf("expected a default rule for status %v", s.ID)
		}
		if r.AssigneeMemberID != creator.ID {
			t.Fatalf("expected default assignee to be the human creator %v, got %v", creator.ID, r.AssigneeMemberID)
		}
	}
}

func TestCreateWorkflow_SeedsDefaultStatusRules_FallsBackToFirstHumanWhenCreatorIsAgent(t *testing.T) {
	f := newFixture()
	agentActorID := uuid.New()
	agent := f.addAgentMember(f.projectID)
	agent.UserID = agentActorID // the API key's owner user id, per setAPIKeyAuthContext

	firstHuman := f.addMember(f.projectID)
	f.addMember(f.projectID) // a second human, to prove "first" (insertion order) is honored

	status := f.addStatus(f.projectID, taskdom.StatusCategoryTodo)

	w, err := f.svc.CreateWorkflow(context.Background(), workflowdom.CreateWorkflowInput{
		ProjectID: f.projectID, Name: "wf", CreatedBy: &agentActorID, AgentID: agent.AgentID,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if w.CreatedBy == nil || *w.CreatedBy != agent.ID {
		t.Fatalf("expected workflow CreatedBy to resolve to the agent member %v, got %+v", agent.ID, w.CreatedBy)
	}

	rules, err := f.repo.ListStatusRulesByWorkflow(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("ListStatusRulesByWorkflow: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 default status rule, got %d", len(rules))
	}
	if rules[0].StatusID != status.ID {
		t.Fatalf("expected the default rule to target the project's only status")
	}
	if rules[0].AssigneeMemberID != firstHuman.ID {
		t.Fatalf("expected default assignee to fall back to the first human member %v, got %v", firstHuman.ID, rules[0].AssigneeMemberID)
	}
}

func TestCreateWorkflow_SeedsDefaultStatusRules_FallsBackWhenNoCreatorInfo(t *testing.T) {
	f := newFixture()
	firstHuman := f.addMember(f.projectID)
	f.addStatus(f.projectID, taskdom.StatusCategoryTodo)

	w := mustCreateWorkflow(t, f)

	rules, err := f.repo.ListStatusRulesByWorkflow(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("ListStatusRulesByWorkflow: %v", err)
	}
	if len(rules) != 1 || rules[0].AssigneeMemberID != firstHuman.ID {
		t.Fatalf("expected default rule assigned to the first human member %v, got %+v", firstHuman.ID, rules)
	}
}

func TestCreateWorkflow_SkipsDefaultStatusRules_WhenNoHumanMemberExists(t *testing.T) {
	f := newFixture()
	f.addAgentMember(f.projectID)
	f.addStatus(f.projectID, taskdom.StatusCategoryTodo)

	w := mustCreateWorkflow(t, f)

	rules, err := f.repo.ListStatusRulesByWorkflow(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("ListStatusRulesByWorkflow: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected no default status rules when no human member exists, got %d", len(rules))
	}
}

func TestListWorkflowsForTask_ReturnsWorkflowsContainingTheTask(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	task := f.addTask(f.projectID)
	otherTask := f.addTask(f.projectID)

	wA := mustCreateWorkflow(t, f)
	wB := mustCreateWorkflow(t, f)
	wC := mustCreateWorkflow(t, f) // does not contain task at all

	if _, err := f.svc.AddNode(ctx, f.projectID, wA.ID, workflowdom.AddNodeInput{TaskID: task.ID}); err != nil {
		t.Fatalf("AddNode wA: %v", err)
	}
	if _, err := f.svc.AddNode(ctx, f.projectID, wB.ID, workflowdom.AddNodeInput{TaskID: task.ID}); err != nil {
		t.Fatalf("AddNode wB: %v", err)
	}
	if _, err := f.svc.AddNode(ctx, f.projectID, wC.ID, workflowdom.AddNodeInput{TaskID: otherTask.ID}); err != nil {
		t.Fatalf("AddNode wC: %v", err)
	}

	got, err := f.svc.ListWorkflowsForTask(ctx, f.projectID, task.ID)
	if err != nil {
		t.Fatalf("ListWorkflowsForTask: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 workflows containing the task, got %d", len(got))
	}
	gotIDs := map[uuid.UUID]bool{}
	for _, w := range got {
		gotIDs[w.ID] = true
	}
	if !gotIDs[wA.ID] || !gotIDs[wB.ID] {
		t.Fatalf("expected wA and wB in results, got %+v", got)
	}
	if gotIDs[wC.ID] {
		t.Fatalf("did not expect wC (doesn't contain the task) in results")
	}
}

func TestListWorkflowsForTask_EmptyWhenTaskNotInAnyWorkflow(t *testing.T) {
	f := newFixture()
	task := f.addTask(f.projectID)

	got, err := f.svc.ListWorkflowsForTask(context.Background(), f.projectID, task.ID)
	if err != nil {
		t.Fatalf("ListWorkflowsForTask: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no workflows, got %d", len(got))
	}
}
