package projectsvc

import (
	"context"
	"fmt"
	"strings"

	plugindom "github.com/Paca-AI/api/internal/domain/plugin"
	projectdom "github.com/Paca-AI/api/internal/domain/project"
	"github.com/Paca-AI/api/internal/platform/authz"
)

// PluginPermissionSource lists the installed plugins whose manifests may
// declare custom permission keys. It is deliberately the narrowest slice of
// plugindom.PluginRepository the project service needs, so wiring it does not
// drag the whole plugin aggregate into this package's dependencies.
type PluginPermissionSource interface {
	List(ctx context.Context) ([]*plugindom.Plugin, error)
}

// WithPluginPermissions wires the source of plugin-declared permission keys.
//
// Without it the service only knows the built-in vocabulary, which would
// reject every role carrying a plugin permission. Production wiring lives in
// internal/bootstrap/app.go; tests that never touch plugin permissions may
// leave it nil.
func (s *Service) WithPluginPermissions(src PluginPermissionSource) *Service {
	s.pluginPerms = src
	return s
}

// pluginKeyNamespace mirrors plugindom's derivation of a plugin's permission
// namespace from its reverse-DNS ID: the last dot-separated segment, with
// hyphens turned into underscores ("com.paca.time-logging" -> "time_logging").
// plugindom keeps its own copy unexported; duplicating four lines here is
// cheaper than widening that package's API, and the plugin manifest's own
// Validate() is what actually enforces the rule at install time.
func pluginKeyNamespace(pluginID string) string {
	parts := strings.Split(pluginID, ".")
	last := parts[len(parts)-1]
	return strings.ReplaceAll(last, "-", "_")
}

// knownPermissionKeys returns the full vocabulary a project role may draw
// from: the host's built-in keys unioned with the custom permission keys
// declared by the installed plugins.
//
// Two notes on the plugin half:
//
//   - ALL installed plugins are consulted, enabled or not. A disabled plugin
//     must not make the roles that reference it uneditable — otherwise turning
//     a plugin off would brick every role an admin had already granted.
//   - Alongside each declared key, the plugin's family wildcard
//     ("time_logging.*") is accepted, because the ceiling check expands
//     "prefix.*" generically and an admin granting a whole plugin surface is
//     doing something meaningful, not something misspelled.
//
// An error from the plugin source is returned, not swallowed: answering "that
// key is unknown" while we could not read half the vocabulary would be the
// same silent-rejection failure this change exists to remove.
func (s *Service) knownPermissionKeys(ctx context.Context) (map[string]struct{}, error) {
	known := make(map[string]struct{}, len(authz.AllPermissions())+8)
	for _, p := range authz.AllPermissions() {
		known[string(p)] = struct{}{}
	}

	if s.pluginPerms == nil {
		return known, nil
	}

	plugins, err := s.pluginPerms.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("project: list plugin permissions: %w", err)
	}
	for _, p := range plugins {
		if p == nil {
			continue
		}
		id := p.Manifest.ID
		if id == "" {
			id = p.Name
		}
		for _, cp := range p.Manifest.CustomPermissions {
			key := strings.TrimSpace(cp.Key)
			if key == "" {
				continue
			}
			known[key] = struct{}{}
		}
		if ns := pluginKeyNamespace(id); ns != "" {
			known[ns+".*"] = struct{}{}
		}
	}
	return known, nil
}

// validateRolePermissions rejects permission keys that belong to no known
// vocabulary, returning an *projectdom.UnknownPermissionsError naming them.
//
// Only GRANTED keys are checked, using the same truthiness rule the grant
// ceiling applies (authz.PermissionsFromMap): an explicit {"x": false} grants
// nothing, so it cannot make a role unassignable and is none of our business.
//
// grandfathered holds keys the stored role already carried. They are skipped,
// so a role written before this check existed stays editable: an admin can
// rename it, or strip the junk key, without first having to guess which of its
// keys the host no longer recognises. New keys are always checked, so a bad
// key can never be ADDED again — the set of dead roles can only shrink.
func (s *Service) validateRolePermissions(ctx context.Context, perms map[string]any, grandfathered map[string]struct{}) error {
	granted := authz.PermissionsFromMap(perms)
	if len(granted) == 0 {
		return nil
	}

	var unknown []string
	var known map[string]struct{}
	for _, p := range granted {
		key := string(p)
		if _, ok := grandfathered[key]; ok {
			continue
		}
		if known == nil {
			var err error
			if known, err = s.knownPermissionKeys(ctx); err != nil {
				return err
			}
		}
		if _, ok := known[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		return projectdom.NewUnknownPermissionsError(unknown)
	}
	return nil
}

// grantedKeySet returns the granted keys of an existing role as a set, for use
// as the grandfathered set on update.
func grantedKeySet(perms map[string]any) map[string]struct{} {
	granted := authz.PermissionsFromMap(perms)
	out := make(map[string]struct{}, len(granted))
	for _, p := range granted {
		out[string(p)] = struct{}{}
	}
	return out
}
