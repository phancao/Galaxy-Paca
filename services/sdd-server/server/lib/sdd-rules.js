/**
 * @file Loads and caches the Galaxy Spec-Driven Development classification rules
 * (server/config/sdd-rules.json, overridable via SDD_RULES_PATH). Exposes the
 * parsed rule set plus a tiny glob matcher used to detect Shared Core file
 * touches. Kept thin and pure so sdd-classify.js stays testable.
 */

const fs = require("fs");
const path = require("path");

const DEFAULT_PATH = path.join(__dirname, "..", "config", "sdd-rules.json");

let cache = null;

/** Convert a simple glob ("**", "*", "{a,b}") into an anchored RegExp. */
function globToRegExp(glob) {
  let re = "";
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === "*") {
      if (glob[i + 1] === "*") {
        // ** matches across path separators
        re += ".*";
        i++;
        if (glob[i + 1] === "/") i++; // collapse **/ → .*
      } else {
        re += "[^/]*";
      }
    } else if (c === "{") {
      const end = glob.indexOf("}", i);
      if (end === -1) {
        re += "\\{";
      } else {
        const opts = glob
          .slice(i + 1, end)
          .split(",")
          .map((o) => o.replace(/[.+^${}()|[\]\\]/g, "\\$&"));
        re += "(?:" + opts.join("|") + ")";
        i = end;
      }
    } else if (".+^$()|[]\\".includes(c)) {
      re += "\\" + c;
    } else {
      re += c;
    }
  }
  return new RegExp("^" + re + "$", "i");
}

function compile(raw) {
  const phaseIndex = {};
  (raw.phases || []).forEach((p, i) => {
    phaseIndex[p.key] = i;
  });
  return {
    raw,
    phases: raw.phases || [],
    phaseIndex,
    commandPhase: raw.commandPhase || [],
    lifecycleTools: raw.lifecycleTools || [],
    toolPhase: raw.toolPhase || [],
    commandPhasePatterns: (raw.commandPhasePatterns || []).map((r) => ({
      re: new RegExp(r.regex, "i"),
      phase: r.phase,
    })),
    readOnlyTools: new Set((raw.readOnlyTools || []).map((t) => t.toLowerCase())),
    level4CommandPatterns: (raw.level4CommandPatterns || []).map((p) => new RegExp(p, "i")),
    level3Lifecycle: new Set((raw.level3Indicators && raw.level3Indicators.lifecycleTools) || []),
    level3CommandPatterns: (
      (raw.level3Indicators && raw.level3Indicators.commandPatterns) ||
      []
    ).map((p) => new RegExp(p, "i")),
    sharedCoreGlobs: (raw.sharedCoreGlobs || []).map(globToRegExp),
  };
}

/** Load (and memoize) the rule set. Falls back to an empty-but-valid set on error. */
function loadRules() {
  if (cache) return cache;
  const file = process.env.SDD_RULES_PATH || DEFAULT_PATH;
  try {
    const raw = JSON.parse(fs.readFileSync(file, "utf8"));
    cache = compile(raw);
  } catch (err) {
    console.warn("[SDD] failed to load rules, using empty set:", err && err.message);
    cache = compile({ phases: [] });
  }
  return cache;
}

/** Test a file path against the Shared Core globs. */
function isSharedCore(filePath, rules) {
  if (!filePath) return false;
  const norm = String(filePath).replace(/\\/g, "/");
  return rules.sharedCoreGlobs.some((re) => re.test(norm));
}

/** Reset memoized rules (tests). */
function _reset() {
  cache = null;
}

module.exports = { loadRules, isSharedCore, globToRegExp, _reset };
