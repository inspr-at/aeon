#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
# AEON-132: preview first; apply only on a clean CR1 branch.
set -Eeuo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"
mode=${1:---dry-run}
case "$mode" in
    --dry-run|--apply) ;;
    *) printf 'usage: %s [--dry-run|--apply]\n' "$0" >&2; exit 2 ;;
esac

if [[ "$mode" == --apply ]]; then
    [[ $(git branch --show-current) == cr1.rehearsal ]] || {
        printf 'apply requires cr1.rehearsal checkout\n' >&2; exit 1;
    }
    [[ -z $(git status --porcelain) ]] || {
        printf 'apply requires a clean checkout; commit the CR1 script first\n' >&2; exit 1;
    }
    [[ -z $(git branch --list cr1.naming-sweep) ]] || {
        printf 'cr1.naming-sweep already exists; inspect it instead of overwriting\n' >&2; exit 1;
    }
    git switch -c cr1.naming-sweep
fi

python3 - "$mode" <<'PY'
import difflib
from pathlib import Path
import subprocess
import sys

apply = sys.argv[1] == "--apply"
tracked = subprocess.check_output(["git", "ls-files", "-z"]).split(b"\0")
changed = 0


def swept(path, content):
    if Path(path).name == "brand.json" or path.startswith("scripts/cutover/"):
        return content
    out = content.replace("github.com/inspr-at/aeon", "github.com/inspr-at/paimos")
    if path == "Dockerfile":
        out = out.replace("-o /aeon ", "-o /paimos ")
        out = out.replace("COPY --from=build /aeon /aeon", "COPY --from=build /paimos /paimos")
        out = out.replace('"/aeon", "serve"', '"/paimos", "serve"')
    if path == "justfile" or path.startswith(".github/workflows/"):
        out = out.replace("bin/aeon", "bin/paimos")
    if path == ".github/workflows/release.yml":
        out = out.replace("aeon-agentd", "paimos-agentd")
    if path.endswith((".go", ".md", ".txt")) or path == "justfile":
        for verb in ("serve", "import", "tenant", "agent-key", "files", "principal"):
            out = out.replace("aeon " + verb, "paimos " + verb)
    if path.endswith(".md"):
        out = out.replace("`aeon`", "`paimos`")
    if path == "cmd/aeon-agentd/main.go":
        out = out.replace("usage: aeon-agentd", "usage: paimos-agentd")
    if path == "cmd/aeon-agentd/doc.go":
        out = out.replace("aeon-agentd", "paimos-agentd")
        out = out.replace("cmd/paimos-agentd", "cmd/aeon-agentd")
    return out


for raw in tracked:
    if not raw:
        continue
    name = raw.decode()
    path = Path(name)
    if not path.is_file() or path.is_symlink():
        continue
    data = path.read_bytes()
    if b"\0" in data:
        continue
    try:
        before = data.decode("utf-8")
    except UnicodeDecodeError:
        continue
    after = swept(name, before)
    if after == before:
        continue
    changed += 1
    if apply:
        path.write_text(after)
    else:
        sys.stdout.writelines(difflib.unified_diff(
            before.splitlines(keepends=True), after.splitlines(keepends=True),
            fromfile="a/" + name, tofile="b/" + name,
        ))

print(f"naming sweep: {changed} tracked files {'changed' if apply else 'would change'}", file=sys.stderr)
PY

if [[ "$mode" == --apply ]]; then
    git diff --check
    git diff --stat
fi
printf '%s\n' 'Coordinator only, after AEON-43 approval and review: gh repo rename paimos --repo inspr-at/aeon' >&2
