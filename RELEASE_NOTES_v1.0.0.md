# AISH v1.0.0

AISH v1.0 adds transparent token, duration, and optional cost tracking across terminal chat, questions, command tasks, document retrieval, and multi-step agents.

## Added

- Per-request input, output, and total token counts
- Exact usage metadata when supported by the provider
- Clearly labeled estimates when exact counts are unavailable
- Request duration tracking
- Per-agent-task usage totals, including planning, correction, and summary calls
- Session, daily, provider, and all-time summaries
- `/usage` inside interactive chat
- JSON and CSV usage exports
- Optional user-configured cloud cost estimates
- Encrypted usage storage when `AISH_ENCRYPTION_KEY` is set
- `show-usage` modes: `summary`, `always`, and `off`

## Commands

```bash
aish usage
aish usage today
aish usage session NAME
aish usage task ID
aish usage export --format json
aish usage export --format csv --output usage.csv
aish usage reset

aish pricing show
aish pricing set input COST_PER_MILLION
aish pricing set output COST_PER_MILLION
```

AISH stores usage metadata only; it does not duplicate prompts or responses into the usage log.
