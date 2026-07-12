# AISH v0.9.0

This release adds two major capabilities: resumable multi-step agent workflows and local personal knowledge collections.

## Agent mode
- Multi-step plans generated from the current project context
- Per-step approve, skip, pause, or cancel controls
- Saved task state with `agent list`, `show`, `resume`, and `delete`
- Automatic corrected-command suggestions after a failed step
- Current-folder and Git repository context

## Personal knowledge
- Separate knowledge collections for personal, work, study, and projects
- Foreground file watching with automatic re-indexing
- Answers prompted to cite source file paths
- Optional AES-GCM encryption for history, sessions, agent state, collection metadata, and indexes
- Improved DOCX paragraph extraction and existing PDF support through Poppler `pdftotext`

## Important
Agent commands still require review. AISH is not a full operating-system sandbox. Keep `AISH_ENCRYPTION_KEY` safe; encrypted data cannot be recovered without it.
