/**
 * @file Pure classifier for the Galaxy Spec-Driven Development process. Given a
 * single hook event (type, tool, raw data) plus the actor agent's current SDD
 * phase, it derives the SDD dimensions the monitor tracks:
 *   - phase    : which of the 8 SDLC stages the action belongs to
 *   - level    : governance level L1 (read) .. L4 (prod deploy / merge to main)
 *   - lifecycle: spec-version action (publish / diff / list / changelog)
 *   - spec     : the spec doc + version a publish/diff touched
 *   - sharedCoreTouch : did this edit hit a Shared Core / contract / schema path
 *
 * No DB, no IO — sdd-store.js wires this to persistence. Keeping it pure makes
 * the governance rules unit-testable and means a wrong classification can never
 * break hook ingestion.
 */

const { loadRules, isSharedCore } = require("./sdd-rules");

/** Strip an MCP tool prefix ("mcp__server__name") down to the bare tool name. */
function baseToolName(toolName) {
  if (!toolName) return "";
  const parts = String(toolName).split("__");
  return parts[parts.length - 1];
}

function extractFilePath(data) {
  const ti = (data && data.tool_input) || {};
  return ti.file_path || ti.path || ti.notebook_path || ti.filePath || null;
}

function extractCommand(data) {
  const ti = (data && data.tool_input) || {};
  return ti.command || null;
}

/** Pull spec doc id + version label from a wiki spec-version tool's input. */
function extractSpec(data) {
  const ti = (data && data.tool_input) || {};
  const docId = ti.document_id || ti.documentId || ti.doc_id || ti.page || ti.slug || null;
  const version =
    ti.version ||
    ti.label ||
    ti.spec_version ||
    ti.version_label ||
    ti.to ||
    ti.from_version ||
    null;
  if (!docId && !version) return null;
  return { docId: docId || null, version: version != null ? String(version) : null };
}

/**
 * Classify one event.
 * @param {object} args
 * @param {string} args.hookType    e.g. "PreToolUse" | "UserPromptSubmit"
 * @param {string} args.eventType   stored event_type
 * @param {string} args.toolName
 * @param {object} args.data        raw hook data (tool_input, prompt, ...)
 * @param {string|null} args.agentPhase  actor agent's current sdd_phase key (sticky)
 * @param {object} [args.rules]     pre-loaded rule set (defaults to loadRules())
 * @returns {object} classification
 */
function classify({ hookType, eventType, toolName, data, agentPhase, rules }) {
  rules = rules || loadRules();
  const base = baseToolName(toolName).toLowerCase();
  const fullTool = (toolName || "").toLowerCase();
  const filePath = extractFilePath(data);
  const command = extractCommand(data);
  const prompt = (data && (data.prompt || data.message)) || "";

  const out = {
    phase: null,
    phaseKey: null,
    phaseSource: null,
    level: null,
    levelReason: null,
    lifecycle: null,
    spec: null,
    sharedCoreTouch: false,
    filePath: filePath || null,
    command: command || null,
  };

  // --- Phase resolution -------------------------------------------------
  // 1) explicit slash command in a user prompt is the strongest signal
  if (hookType === "UserPromptSubmit" && prompt) {
    const hit = rules.commandPhase.find((c) => prompt.includes(c.match));
    if (hit) {
      out.phaseKey = hit.phase;
      out.phaseSource = "command";
    }
  }
  // 2) certain tools imply a phase (e.g. publishing a spec version → techspec)
  if (!out.phaseKey) {
    const hit = rules.toolPhase.find((t) => fullTool.includes(t.match.toLowerCase()));
    if (hit) {
      out.phaseKey = hit.phase;
      out.phaseSource = "tool";
    }
  }
  // 3) a bash command pattern (running tests → verify, deploy → release)
  if (!out.phaseKey && command) {
    const hit = rules.commandPhasePatterns.find((p) => p.re.test(command));
    if (hit) {
      out.phaseKey = hit.phase;
      out.phaseSource = "tool";
    }
  }
  // 4) otherwise inherit the agent's sticky phase
  if (!out.phaseKey && agentPhase) {
    out.phaseKey = agentPhase;
    out.phaseSource = "inherit";
  }
  if (out.phaseKey != null && rules.phaseIndex[out.phaseKey] != null) {
    out.phase = rules.phaseIndex[out.phaseKey];
  }

  // --- Spec lifecycle ---------------------------------------------------
  const lc = rules.lifecycleTools.find(
    (l) => fullTool.includes(l.match.toLowerCase()) || base === l.match
  );
  if (lc) {
    out.lifecycle = lc.lifecycle;
    out.spec = extractSpec(data);
  }

  // --- Shared Core touch ------------------------------------------------
  out.sharedCoreTouch = isSharedCore(filePath, rules);

  // --- Governance level -------------------------------------------------
  // Only action-bearing events carry a meaningful level. UserPromptSubmit /
  // Stop / Notification are process signals, not governed actions.
  const isAction = hookType === "PreToolUse" || hookType === "PostToolUse";
  if (isAction) {
    const readOnly = rules.readOnlyTools.has(base) || rules.readOnlyTools.has(fullTool);
    // L4 — merge to main / production deploy
    const isL4 = command && rules.level4CommandPatterns.some((re) => re.test(command));
    // L3 — Shared Core / contract / schema, publishing a shared spec, or a migration
    const isL3 =
      out.sharedCoreTouch ||
      (lc && rules.level3Lifecycle.has(lc.match)) ||
      (command && rules.level3CommandPatterns.some((re) => re.test(command)));

    // Reading/analyzing is always L1 — even on Shared Core. Governance only
    // escalates for actions that WRITE. So read-only is checked first.
    if (readOnly) {
      out.level = 1;
      out.levelReason = "read-only";
    } else if (isL4) {
      out.level = 4;
      out.levelReason = "merge-to-main / production deploy";
    } else if (isL3) {
      out.level = 3;
      out.levelReason = out.sharedCoreTouch
        ? "touches Shared Core / contract / schema"
        : lc
          ? "publishes a shared spec version"
          : "database migration";
    } else {
      out.level = 2;
      out.levelReason = "edits within a working branch";
    }
  }

  return out;
}

module.exports = { classify, baseToolName, extractSpec };
