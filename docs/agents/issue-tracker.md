# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues. Use the `gh` CLI for all operations.

## Conventions

- **Create an issue**: `gh issue create --repo its-a-unixsystem/scopeskill --title "..." --body "..."`
- **Read an issue**: `gh issue view <number> --repo its-a-unixsystem/scopeskill --comments`, filtering comments by `jq` and also fetching labels.
- **List issues**: `gh issue list --repo its-a-unixsystem/scopeskill --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` with appropriate `--label` and `--state` filters.
- **Comment on an issue**: `gh issue comment <number> --repo its-a-unixsystem/scopeskill --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --repo its-a-unixsystem/scopeskill --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --repo its-a-unixsystem/scopeskill --comment "..."`

Infer the repo from `git remote -v` when possible. Use the explicit `--repo its-a-unixsystem/scopeskill` form when running outside this clone.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --repo its-a-unixsystem/scopeskill --comments`.
