# AISH v0.9.1

This patch improves agent-mode reliability.

## Fixed
- Prevented invalid `go mod init -v` plans for existing Go projects.
- Added a trusted plan for inspecting and testing Go projects.
- Stopped automatic correction from retrying the same broken command forever.
- Ctrl+C now saves an active task as paused for later resume.
- `agent show`, `resume`, and `delete` now accept list numbers such as `1`.
- Existing knowledge collections can be selected again without an error.
- Added a concise AI summary after successful agent completion.
