# tokfresh

> Automate your Claude Pro/Max token reset timing via Cloudflare Workers

Claude Pro/Max usage resets every 5 hours from your first API call. TokFresh deploys a Cloudflare Worker to your account that pre-triggers the timer on a cron schedule, so resets align with your workday.

## Install

```sh
brew install stevejkang/tap/tokfresh   # macOS/Linux
scoop install tokfresh                  # Windows (after: scoop bucket add stevejkang https://github.com/stevejkang/scoop-bucket)
go install github.com/stevejkang/tokfresh-cli@latest  # or via Go
```

Or download a binary from [GitHub Releases](https://github.com/stevejkang/tokfresh-cli/releases).

## Quick Start

```sh
tokfresh init      # interactive setup: Claude OAuth → schedule → Cloudflare deploy
tokfresh status    # view all managed instances
```

## Commands

| Command | Description |
|---------|-------------|
| `tokfresh init` | Interactive setup wizard |
| `tokfresh status` | List all managed workers |
| `tokfresh update <name>` | Change schedule or notifications |
| `tokfresh upgrade` | Re-deploy all workers with latest template |
| `tokfresh remove <name>` | Delete a worker |
| `tokfresh logs <name>` | Stream real-time worker logs |
| `tokfresh test <name>` | Trigger worker once manually |
| `tokfresh version` | Print version info |

Use `-v` for info-level logs, `-vv` or `--debug` for full HTTP traces.

## How It Works

1. `tokfresh init` opens a browser for Claude OAuth, you paste the code
2. CLI deploys a Worker with: KV namespace (refresh token), cron schedule (4x/day at 5h intervals), secrets
3. Worker runs on cron: refreshes Claude token, calls API to start the timer, rotates token in KV
4. Your refresh token **never leaves your Cloudflare account**

## Multi-Account

One Cloudflare account can manage multiple Claude accounts. Each `tokfresh init` creates a separate worker with its own KV namespace:

```sh
tokfresh init   # → tokfresh-scheduler-work
tokfresh init   # → tokfresh-scheduler-personal
tokfresh status # shows both
```

## License

MIT
