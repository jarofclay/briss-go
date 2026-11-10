# 0007 No credentials in the repository

Status: accepted 2026-11-05.

**Decision.** This is a public fork. API keys and other credentials are never committed,
not even in ignored files. The app reads the Claude key from the OS keychain or
`ANTHROPIC_API_KEY`; tests and spikes read the environment only; CI uses repository
secrets. The root `.gitignore` excludes `.env`, `.env.*`, `*.env`, `*.key` and
`secrets/`. GitHub secret scanning with push protection stays enabled. Since phase 2,
CI runs gitleaks on the full history and the tree with `.gitleaks.toml`, and
`.pre-commit-config.yaml` offers the same scan as a local hook. The config adds a rule for
every Anthropic key kind: gitleaks' default rule matches only `sk-ant-api03-` keys and
missed a planted `sk-ant-usr-` key, the format of the key once pasted into a session.

**Why.** A test key was once pasted into a chat session; it was used only through the
environment, never written to the repository, and treated as revoked afterwards.
