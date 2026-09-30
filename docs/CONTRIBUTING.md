# Development and Commit Guidelines

## Commit Messages

All commit subjects and bodies must be written in English and follow Conventional Commits:

```text
<type>(<optional scope>): <description>
```

Use an imperative, concise description. Supported types include feat, fix, refactor, test, docs, build, ci, chore, perf and revert. Use ! or a BREAKING CHANGE footer when applicable.

Examples:

```text
chore: initialize KeyVivi repository
feat(display): add FIFO keystroke queue
fix(keyboard): preserve held modifier state
test(platform): verify hotkey cleanup
```

Keep commits focused. Inspect staged changes before committing; never include generated executables, desktop screenshots, secrets, or unrelated changes. Do not fabricate author identity or rewrite history without approval.

## Layout

Application entry points belong in cmd/keyvivi. Implementation belongs in internal modules. All tests belong in tests/unit or tests/integration. Project documents belong in docs, binaries in dist and local test captures in tests/artifacts.

## Verification

Run commands from the repository root:

```bash
go test -race ./...
go test -race -tags integration ./tests/integration
go build -ldflags="-s -w -H=windowsgui" -o dist/KeyVivi.exe ./cmd/keyvivi
```

Windows integration tests require an interactive desktop and inject keyboard events. Run them explicitly; do not mark desktop acceptance complete based only on a successful build.
