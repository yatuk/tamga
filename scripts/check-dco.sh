#!/usr/bin/env bash
# check-dco.sh — check that every commit in a range carries a Developer
# Certificate of Origin sign-off (https://developercertificate.org/).
#
#   scripts/check-dco.sh <base> [<head>]
#   scripts/check-dco.sh upstream/development      # your branch, before a push
#
# A commit passes when one of its Signed-off-by trailers names the commit's
# author email (compared without regard to case). Merge commits are skipped,
# and so are commits authored by dependabot[bot]. Called from
# .github/workflows/dco.yml with the pull request's base and head.

set -euo pipefail

base="${1:?usage: check-dco.sh <base> [<head>]}"
head="${2:-HEAD}"

commits="$(git rev-list --no-merges --reverse "${base}..${head}")"
if [ -z "$commits" ]; then
    echo "check-dco: no commits in ${base}..${head}"
    exit 0
fi

checked=0
skipped=0
missing=0
for sha in $commits; do
    author_name="$(git log -1 --format='%an' "$sha")"
    author_email="$(git log -1 --format='%ae' "$sha")"
    subject="$(git log -1 --format='%s' "$sha")"
    short="$(git rev-parse --short "$sha")"

    case "$author_name" in
        "dependabot[bot]")
            skipped=$((skipped + 1))
            continue
            ;;
    esac
    checked=$((checked + 1))

    want="$(printf '%s' "$author_email" | tr '[:upper:]' '[:lower:]')"
    found=0
    while IFS= read -r value; do
        [ -n "$value" ] || continue
        # "Name <email>": keep what is between the angle brackets.
        email="${value##*<}"
        email="${email%%>*}"
        email="$(printf '%s' "$email" | tr '[:upper:]' '[:lower:]')"
        if [ "$email" = "$want" ]; then
            found=1
            break
        fi
    done < <(git log -1 --format='%(trailers:key=Signed-off-by,valueonly,unfold)' "$sha")

    if [ "$found" -eq 0 ]; then
        missing=$((missing + 1))
        msg="${short} ${subject}: no Signed-off-by for the author <${author_email}>"
        if [ -n "${GITHUB_ACTIONS:-}" ]; then
            echo "::error title=DCO sign-off missing::${msg}"
        else
            echo "MISSING: ${msg}"
        fi
    fi
done

echo "check-dco: ${checked} commit(s) checked, ${skipped} skipped (dependabot), ${missing} without a matching sign-off"

if [ "$missing" -gt 0 ]; then
    cat <<'EOF'

Every commit needs a "Signed-off-by: Your Name <you@example.com>" line whose
email is the commit's author email. To add it:

  the last commit:      git commit --amend --no-edit -s
  every commit on the branch, rebased onto this repository's development
  branch (the remote may be called origin or upstream in your clone):
                        git rebase --signoff upstream/development

then push again with: git push --force-with-lease

If the author email is wrong rather than the sign-off missing, set
git config user.email first and add --reset-author to the amend.
See CONTRIBUTING.md, "Sign your commits (DCO)".
EOF
    exit 1
fi
