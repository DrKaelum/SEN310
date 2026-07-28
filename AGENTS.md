# Repository workflow

- After fully completing and verifying a task, if this working directory is in a Git repository with a GitHub remote, commit only the changes made for that task and push them to GitHub.
- Always make task commits on a branch named `codex`. If `codex` already exists, switch to it before committing; otherwise, create it and commit there.
- Use a lightweight Conventional Commits format for commit subjects: `type: concise description` or, when useful, `type(scope): concise description`. Prefer lowercase types such as `feat`, `fix`, `refactor`, `build`, `chore`, `docs`, `test`, `style`, `perf`, `ci`, and `revert`, selecting whichever accurately describes the task. Keep the description clear and consistent with the repository's existing straightforward commit style; do not require optional Conventional Commits ceremony such as scopes, bodies, footers, or breaking-change notation when it adds no value.
- Preserve unrelated user changes in the working tree; do not include them in the task's commit.
- If committing or pushing is unsafe, blocked, or would require a consequential choice, explain the issue to the user instead of silently skipping submission.
