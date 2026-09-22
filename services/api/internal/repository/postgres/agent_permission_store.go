package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Paca-AI/api/internal/platform/authz"
)

// ListAgentProjectPermissions returns permissions from project role memberships for
// an agent in the given project.
func (s *AuthzPermissionStore) ListAgentProjectPermissions(ctx context.Context, agentID, projectID uuid.UUID) ([]authz.Permission, error) {
	var rows []struct {
		Permissions []byte `db:"permissions"`
	}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT pr.permissions
		FROM project_roles pr
		JOIN project_members pm ON pm.project_role_id = pr.id
		WHERE pm.agent_id = $1 AND pm.project_id = $2 AND pm.deleted_at IS NULL`,
		agentID.String(), projectID.String())
	if err != nil {
		return nil, fmt.Errorf("authz store: list agent project permissions: %w", err)
	}

	return collectPermissions(rows), nil
}
