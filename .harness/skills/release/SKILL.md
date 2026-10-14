---
name: release
description: Cut a gascoigne release - version bump, changelog, tag, push, website page, latest.json. Use when asked to prepare, publish or ship a release (e.g. "release 0.6.2").
---

# Release

Two repos: this one (`gascoigne-cli`, remote `gh` = GitHub, `origin` = local gitea) and the website
`../gascoigne-webpage` (remote `origin` = `gascoigne-ai/gascoigne-ai.github.io`, branch `main`).
Pushing a tag and the website are public: do them only when the user asked for a release.
Versions follow SemVer; a patch release carries fixes and small features, a minor one new capability.

## 1. Prepare (this repo)
1. Work tree clean, on `master`; `git fetch gh` and check `gh/master` has nothing you lack.
2. `make check` passes. Run `make eval` too if an optimizer changed.
3. `CHANGELOG.md`: add `## [X.Y.Z] - YYYY-MM-DD` above the previous release (Keep a Changelog:
   Added / Changed / Fixed). One summary sentence under the heading. Name data migrations and
   changed defaults (retention, config) so upgrading users see them. The release workflow uses this
   section as the GitHub release notes and fails without it.
4. `version/version.go`: `const Number = "X.Y.Z"`. The workflow fails when the tag differs.
5. Commit both as `release: X.Y.Z <short summary>` (fixes go in their own earlier commits).

## 2. Tag and push
```sh
git tag vX.Y.Z                      # lightweight, like the earlier tags
git push gh master vX.Y.Z           # not origin: that is the local gitea mirror
gh run list --repo gascoigne-ai/gascoigne-cli --workflow Release --limit 1
```
The `Release` workflow (`.github/workflows/release.yml`) checks tag = `version.Number`, runs
`make check`, builds linux/darwin/windows × amd64/arm64 archives plus `checksums.txt` (7 assets)
and creates the release from the changelog section. Wait for `completed success` and check
`gh release view vX.Y.Z --json assets --jq '.assets|length'` is 7. A failed run: fix, delete the tag
(`git push gh :vX.Y.Z`, `git tag -d vX.Y.Z`), release again; or rerun with the workflow's `tag` input
for an existing tag.

## 3. Website (`../gascoigne-webpage`, see its README)
Order matters: `latest.json` is what sends users to the release page, so it comes last.
1. Add `releases/X.Y.Z/index.html` (copy the previous page: hero, what's new, upgrade steps, links to
   `releases/tag/vX.Y.Z`) and a changelog entry in `index.html` (open the new `<details>`, close the
   old one) and update the nav link.
2. Commit "Add the X.Y.Z release page and changelog entry", push `main`; the `Pages` workflow
   deploys. Check `https://gascoigne-ai.github.io/releases/X.Y.Z/` returns 200.
3. Only then set `latest.json` to `{"id": "X.Y.Z", "released": "YYYY-MM-DD", "page": "releases/X.Y.Z/"}`,
   commit "Point latest.json at X.Y.Z", push, and check `curl https://gascoigne-ai.github.io/latest.json`.
   gascoigne reads it once a day and opens the page once per user at their next agent session.

## 4. After
Report: tag, commit, workflow result, asset count, page URL. Note anything not verified.
