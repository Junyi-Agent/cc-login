#!/usr/bin/env bash
# ship: land the feature branch of this worktree on main in one step (ADR 0014):
#
#   1. check: a feature branch, nothing uncommitted, up to date with
#      origin/main, gh logged in as the repo owner, claude signed in (the
#      review gate runs it);
#   2. push the branch: the pre-push review gate runs;
#   3. open the PR, or reuse the open one;
#   4. wait for CI; stop if a check fails;
#   5. squash merge (GitHub deletes the remote branch);
#   6. remove the worktree and the local branch, fast-forward main.
#
# Usage: scripts/ship.sh merge branch [--keep]
#   merge branch   land this worktree's branch on main, as above
#   --keep         leave the worktree and the branch
set -euo pipefail

say() { echo "ship: $*"; }
die() {
	echo "ship: $*" >&2
	exit 1
}

usage() { sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; }

case "${1:-} ${2:-}" in
"merge branch") shift 2 ;;
"-h "* | "--help "* | "help "*)
	usage
	exit 0
	;;
*)
	usage >&2
	exit 2
	;;
esac

keep=0
case "${1:-}" in
"") ;;
--keep) keep=1 ;;
*) die "unknown option $1; usage: scripts/ship.sh merge branch [--keep]" ;;
esac
[ "$#" -le 1 ] || die "too many arguments; usage: scripts/ship.sh merge branch [--keep]"

# 1. Check before anything leaves the machine.
branch=$(git symbolic-ref --short HEAD 2>/dev/null) || die "not on a branch"
[ "$branch" != main ] || die "on main: run ship in a feature worktree (see CLAUDE.md)"
[ -z "$(git status --porcelain)" ] || die "uncommitted changes: commit them first"

git fetch -q origin main
git merge-base --is-ancestor origin/main HEAD ||
	die "$branch is behind origin/main: git rebase origin/main, then ship again"
[ -n "$(git log --oneline origin/main..HEAD)" ] || die "$branch has no commits beyond origin/main"

owner=$(gh repo view --json owner -q .owner.login) || die "gh cannot read the repo: gh auth status"
account=$(gh api user -q .login) || die "gh is not logged in: gh auth login"
[ "$account" = "$owner" ] || die "gh account is $account, not $owner: gh auth switch -u $owner"

command -v claude >/dev/null && claude auth status >/dev/null 2>&1 ||
	die "the pre-push review gate needs a signed-in claude in PATH (see CLAUDE.md)"

# 2. Push; the pre-push hook runs the review gate.
say "push $branch (the review gate runs)"
git push -u origin "$branch"

# 3. The PR: the open one, or a new one from the commits.
pr=$(gh pr list --head "$branch" --state open --json number -q '.[0].number')
if [ -z "$pr" ]; then
	title=$(git log -1 --format=%s)
	body=$(git log --reverse --format='- %s%n%n%b' origin/main..HEAD)
	gh pr create --base main --head "$branch" --title "$title" --body "$body" >/dev/null
	pr=$(gh pr list --head "$branch" --state open --json number -q '.[0].number')
fi
say "PR #$pr: $(gh pr view "$pr" --json url -q .url)"

# 4. Wait for CI. GitHub takes a few seconds to register the checks.
for _ in $(seq 24); do
	n=$(gh pr checks "$pr" --json name -q length 2>/dev/null || echo 0)
	[ "$n" != 0 ] && break
	sleep 5
done
say "PR #$pr: waiting for CI"
gh pr checks "$pr" --watch --fail-fast --interval 15 ||
	die "a CI check failed; fix it, commit, and ship again"

# 5. Merge.
gh pr merge "$pr" --squash
say "PR #$pr merged"

# 6. Clean up: the worktree and branch, then main.
top=$(git rev-parse --show-toplevel)
repo=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
cd "$repo"

if [ "$keep" = 0 ] && [ "$top" != "$repo" ]; then
	if git worktree remove "$top" && git branch -q -D "$branch"; then
		say "removed $top and branch $branch"
	else
		say "could not remove $top; remove it with: git worktree remove $top && git branch -D $branch"
	fi
fi

if [ "$(git symbolic-ref --short HEAD 2>/dev/null)" = main ] && git pull -q --ff-only; then
	say "main is at $(git log -1 --format='%h %s')"
else
	say "update main yourself: cd $repo && git pull --ff-only"
fi
[ "$keep" = 1 ] || say "the worktree is gone: cd $repo"
