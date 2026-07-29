# cyberstalk-me Windows Agent

A single Windows executable that reports what you're doing to a cyberstalk-me
server. It collects the foreground window, idle time, battery, and network type
on the device, **sanitizes everything locally**, and posts only the sanitized
result at a fixed interval.

## Privacy model (read this first)

The whole product is safe only if the raw window title never leaves the device.
Three rules make that hold:

1. **The raw title is read from the OS only when a rule needs it** — a
   `title_patterns` rule or an `expose_title` opt-in. A deployment with no title
   rules never reads a title at all.
2. **A process that matches no rule reports a generic description**
   ("某个应用 · 使用中"), never its exe name. An exe name can itself leak
   (internal tools, project codenames). Want it shown? Write a rule.
3. **The raw title lives in memory for one mapping call** and never enters the
   report payload, the logs (even `-v`), the `-dry-run` output, or disk.

`expose_title` is the one explicit opt-out: a process listed there reports its
raw window title verbatim as the description. It is empty by default. Only use
it when you really mean it.

## Build

From the repo root:

```bash
go build -o client-windows/agent.exe ./client-windows/cmd/agent
```

(The workspace's bare `go build ./...` does not work here — use the explicit
module path, or `cd client-windows && go build -o agent.exe ./cmd/agent`.)

## Configure

1. Start the server and register this device. The command prints a one-time
   token plus a config snippet you can paste in:

   ```bash
   go run ./server/cmd/server register-device win-desktop 我的台式机 windows
   ```

   The server and `register-device` share one SQLite file (`cyberstalk.db` in
   the working directory by default, or `SQLITE_PATH`), so run both from the
   same directory.

2. Copy `config.example.yaml` to `config.yaml` next to `agent.exe`, paste the
   printed `server_url` / `device_id` / `token` / `interval` block, and add your
   mapping rules.

3. `config.yaml` contains the device token — treat it as a secret. Restrict its
   ACL (file Properties → Security) so only your user can read it. `config.yaml`
   is gitignored; only `config.example.yaml` is tracked.

By default the agent looks for `config.yaml` **next to the executable**, not in
the working directory, so a double-clicked exe and a registry-autostarted exe
both find it.

## Run

```bash
# one-shot: print the sanitized payload that WOULD be sent, no network
./client-windows/agent.exe -config ./client-windows/config.yaml -dry-run

# long-running, verbose
./client-windows/agent.exe -config ./client-windows/config.yaml -v
```

Open the server URL (default <http://localhost:8080>) to see the device card.

Flags:

- `-config <path>` — config file. Default: `config.yaml` next to the exe.
- `-dry-run` — collect + map one cycle, print the sanitized payload JSON to
  stdout, and exit. No network. Use this to verify nothing sensitive leaks.
- `-v` — debug logging (includes each round's mapped `{app, description}`,
  never the raw title or the token).

## Verify nothing leaks (the canary test)

1. Make some window's title unmistakable — e.g. name a browser tab
   `SECRET-TITLE-CANARY`.
2. Run `-dry-run`. The printed payload must NOT contain `SECRET-TITLE-CANARY`
   unless that process is in `expose_title`.
3. Run `-v`. The logs must not contain the canary or the token.
4. On the website, the card must not show the canary.

## Autostart on login (registry Run key)

Add a string value under
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`:

- **Name:** `cyberstalk-agent` (or whatever you like)
- **Value:** `"C:\path\to\agent.exe" -config "C:\path\to\config.yaml"`

Use absolute paths — the working directory is unreliable for an autostarted
process. Remove the value to stop autostart. The agent does not write the
registry itself; that is a decision you make.

## Mapping rules

See `config.example.yaml`. `process` is the exe base name, matched
case-insensitively. `title_patterns` refine the description by matching the raw
title in memory (the title is matched, never reported). `expose_title` lists
processes whose raw title is reported verbatim — dangerous, empty by default.

## Resilience

If the server is unreachable or returns 5xx, the agent backs off exponentially
(interval → 2× → 4× → … capped at 2 minutes) and keeps trying; it never crashes.
A bad token (401) or a malformed request (400) is treated as a config error and
backs off straight to the 2-minute cap. When the server recovers, reporting
resumes automatically. Failed rounds are dropped — only the latest state is
ever sent, so a stale report can't pollute the server's `last_seen`.