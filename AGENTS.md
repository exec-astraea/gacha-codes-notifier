# hoyo-codes — agent guide

A small Go service that watches for new gacha **redemption codes** (Genshin
Impact, Honkai: Star Rail, Zenless Zone Zero, Honkai Impact 3rd, and Wuthering
Waves) and posts them to **Discord webhooks** on a cron schedule. Docker-first,
no database, no framework.

Codes are pulled from up to three APIs and merged/de-duplicated, so a hiccup on
one source doesn't cause a miss. A source is skipped for any game it doesn't
serve (empty slug), and it only errors when *every* attempted source fails:

- [ennead.cc](https://api.ennead.cc/mihoyo) — `/<game>/codes` (HoYoverse only)
- [torikushiii/hoyoverse-api](https://github.com/torikushiii/hoyoverse-api) — `hoyo-codes.seria.moe` (HoYoverse only)
- **OpenGachaCodes** — self-hosted read-only API (`opengachaBaseUrl` config).
  Supplements the HoYo games it also serves and is the **sole** source for
  non-HoYo games like Wuthering Waves. Disabled when `opengachaBaseUrl` is unset.

## Layout

This directory is the whole, self-contained project (Go code, `Dockerfile`,
docs, config). It knows nothing about how it's deployed — a separate
`docker-compose.yaml` outside this dir builds the image and supplies its own
config file and data volume; treat that as external.

```
config.yaml         live config (gitignored — holds webhook URLs)
config.example.yaml committed, fully-commented template
data/               runtime state (sent.json), gitignored
Dockerfile
README.md           dry, user-facing
AGENTS.md           this file
*.go                the service (flat package main)
```

This is a flat `package main`, one concern per file:

| File            | Responsibility                                                          |
| --------------- | ----------------------------------------------------------------------- |
| `main.go`       | Startup, config load, cron loop, the per-check diff/announce.           |
| `config.go`     | YAML config load (`gopkg.in/yaml.v3`) + resolution/validation.          |
| `games.go`      | `games` registry: canonical key → per-source slugs, colour, redeem URL. |
| `sources.go`    | Fetch + normalize from both upstream APIs, merge/de-dupe.               |
| `discord.go`    | Build embeds, chunk to Discord limits, POST the webhook.                |
| `state.go`      | `data/sent.json` load/save (atomic write).                              |
| `util.go`       | Tiny shared helpers.                                                     |
| `discord_test.go` | Unit test for `buildContent` (placeholder/plural expansion).          |

## Build / run / verify

All commands run from this directory.

```sh
go build ./... && go vet ./... && go test ./... && gofmt -l .   # compile + vet + test + fmt gate
docker build -t hoyo-codes .                                    # build the image
```

Quick local run without Docker. Config path comes from `CONFIG_FILE` (default
`/app/config.yaml`); point it at a scratch file:

```sh
CONFIG_FILE=/path/to/test-config.yaml go run .
```

For a one-shot test, set `schedule: "0 0 1 1 *"` (parks cron far in the future so
only the immediate `runOnStart` check fires) and `markExistingOnFirstRun: false`
(forces the active backlog to post; by default the first run marks it sent silently).

After a `go build .` here, delete the stray `hoyo-codes` binary it drops
(already gitignored).

## Configuration reference

All config lives in `config.yaml` (gitignored — it holds webhook URLs). See
`config.example.yaml` for the fully-commented template.

```yaml
schedule: "*/30 * * * *"        # 5-field cron
timezone: "Europe/Amsterdam"    # optional; when the schedule runs
markExistingOnFirstRun: true    # suppress the old-code backlog on first run
runOnStart: true
httpTimeout: 20
maxPostAttempts: 10             # give up announcing a code after N failed checks

games:                          # only games listed here are watched
  genshin:
    webhook: "https://discord.com/api/webhooks/..."
    message: "<@&ROLE_ID> {count} new {if-singular:code}{if-plural:codes} 🎁"
    username: "Genshin Codes"        # optional sender name override
    avatarUrl: "https://…/pic.png"   # optional sender avatar (public image URL)
  hsr:
    webhook: "https://discord.com/api/webhooks/..."   # its own channel
    message: "<@&ROLE_ID> Trailblazers — {count} new {if-singular:code}{if-plural:codes}!"
# honkai3rd:                    # comment a game out (or delete it) to stop watching
#   webhook: "…"
#   message: "…"
```

- **Which games run** = the keys under `games:` (known keys: `genshin`, `hsr`,
  `zzz`, `honkai3rd`). There is no enable/disable flag — comment a game out or
  delete it to stop watching it.
- **Every listed game requires `webhook` and `message`.** There is no shared
  default block — each game is self-contained. Startup fails with a clear error if
  a game is missing either, if no games are configured, or on any unknown key (typos).
- **Optional per-game sender identity** (both fall back if omitted):
  - `username` — the Discord sender name; defaults to `defaultUsername`
    (`"Hoyo Codes"`) in `config.go`.
  - `avatarUrl` — a public image URL (PNG/JPG/GIF) Discord fetches as the sender
    avatar; omit to keep the webhook's own configured avatar. Both map onto the
    webhook payload's `username` / `avatar_url` fields.
- **Message placeholders**:
  - `{count}` — number of new codes.
  - `{if-singular:X}` / `{if-plural:X}` — `X` is kept only when the count is 1
    (singular) or not 1 (plural); the other is dropped. Handles irregular plurals
    and other languages. The text inside must not contain a `}`.

### Pinging a role

To ping a role, put its mention token **in the game's `message`** — the message is
the Discord message body, and mentions only ping from there (never from an embed):

```yaml
message: "<@&123456789012345678> {count} new codes!"   # pings a role
message: "@everyone {count} new codes!"                 # literal
```

Role mentions need the role's **numeric ID**, not its name (Discord → Settings →
Advanced → **Developer Mode**, then right-click the role → **Copy ID**).

## How a check works (`runCheck`)

1. For each watched game, `fetchCodes` queries **every source that serves it**
   (empty-slug sources are skipped) and merges by upper-cased code. One source
   failing is tolerated; only *all* attempted sources failing skips the game.
2. New codes = fetched codes not in `data/sent.json`. They're marked sent
   immediately.
3. First time a game is seen (no state), codes are **marked sent silently** when
   `markExistingOnFirstRun: true` (the default) — avoids dumping the whole
   backlog on boot. Set it false to announce the backlog instead. A game with
   **no** active codes on its first fetch is still seeded (`state.seed`) so the
   next fetch that finds codes is announced as new rather than re-treated as
   first-run backlog.
4. New codes become one Discord embed (with a one-click **Redeem** link where the
   game supports web redemption), posted to **that game's** webhook
   (`cfg.Notify[key]`). `buildContent` expands `{count}` and resolves the
   `{if-singular:X}`/`{if-plural:X}` blocks in the game's `message`, then sends it
   as the message `content`; a `<@&ROLE_ID>` token pings from there.
5. Codes are marked sent **only after their announce post succeeds** (state is
   saved once at the end of the check). A failed post leaves its codes unmarked,
   so the next check retries them — nothing reached Discord, so this re-tries
   rather than double-posts, and a broken webhook self-heals once fixed instead
   of silently dropping codes. A transient `429` is first retried in-place per
   its `Retry-After` header (`postPayload`, bounded to 3). Force a re-announce of
   already-sent codes by deleting `data/sent.json`.
   - **Retry cap:** each failed check bumps a per-code counter in `state.Attempts`
     (`recordAttempt`). Once a code reaches `maxPostAttempts` (config, default 10)
     it's marked done and **given up on** (never announced) so a persistently
     failing post can't retry/log forever. The counter is per-code, so a genuinely
     new code arriving mid-retry gets its own fresh budget; `mark` clears a code's
     counter on success or give-up.
   - **Edge case:** if one check yields enough new codes to span multiple Discord
     messages (>25 at once) and an early message succeeds but a later one fails,
     the successful ones can repost on retry. HoYo drops codes a few at a time, so
     in practice it's one message per game and this can't arise.

## Config internals (`config.go`)

Parsed with `gopkg.in/yaml.v3` in strict mode (`KnownFields(true)` — unknown keys
are a fatal error, catching typos). `loadConfig` resolves the file into `Config`:

- Top-level scalars (`schedule`, `timezone`, `markExistingOnFirstRun`,
  `runOnStart`, `httpTimeout`, `maxPostAttempts`, `dataDir`) have defaults via
  `orString/orBool/orInt`.
- Watched games = the keys present under `games:` (iterated in `gameOrder` for
  stable order). Unknown keys warn and are skipped; there is no disable flag.
- Each game is self-contained (`GameNotify{Webhook, Message, Username, AvatarURL}`;
  `Username` defaults to `defaultUsername`, `AvatarURL` is optional). `loadConfig`
  collects validation problems and returns them together: fatal if no games are
  configured, any listed game is missing `webhook` or `message`, or a listed game
  has no reachable source (OpenGachaCodes-only with no `opengachaBaseUrl`).

When adding a config knob: add it to `fileConf`, resolve/validate it in
`loadConfig`, and document it in `config.example.yaml` and this file.

## Upstream APIs

- **ennead** — `https://api.ennead.cc/mihoyo/<slug>/codes`, slugs `genshin`,
  `starrail`, `zenless`, `honkai`. Shape: `{"active":[{"code","rewards":[...]}],"inactive":[...]}`.
- **torikushiii** — `https://hoyo-codes.seria.moe/codes?game=<slug>`, slugs
  `genshin`, `hkrpg`, `nap`, `honkai3rd`. Shape: `{"codes":[{"code","rewards":"A*60;B*5"}]}`
  (active only). Rewards get normalized (`A*60;B*5` → `A ×60, B ×5`).
- **OpenGachaCodes** — `<opengachaBaseUrl>/games/<slug>/codes`, slugs `genshin`,
  `starrail`, `zenless`, `wuwa` (also endfield/nte, not yet in the registry).
  Shape: a flat array `[{"code","rewards":["Astrite x50", ...]}]` (active only).
  Self-hosted, no auth/CORS, GET-only, strict paths (no trailing slash). A `404`
  is treated as "nothing to contribute" (not a source failure). Reward quantities
  use ASCII `x`, not `×`; they're joined with `, ` as-is. `fetchOpengacha` lives
  in `sources.go`; the base URL is `opengachaBaseUrl` in config.

The internal key differs from the torikushiii slug for Star Rail (`hsr`/`hkrpg`)
and Zenless (`zzz`/`nap`). Per-source slugs live in `games.go`; a `Game` sets a
slug to `""` for any source that doesn't serve it (e.g. `wuwa` has empty
`Ennead`/`Tori` and is OpenGachaCodes-only), and `fetchCodes` skips empty-slug
sources. To add another game, add one entry to the `games` map **and** to
`gameOrder`; nothing else needs to change. A game whose only source is
OpenGachaCodes **fails startup** if `opengachaBaseUrl` is unset (it could never
produce codes) — a validation problem in `loadConfig`, like a missing webhook.

## Conventions

- **Config is a YAML file.** Add new knobs to `fileConf`/`Config` in `config.go`
  and document them in `config.example.yaml` and this file. Every listed game
  must define its own `webhook` and `message` (validated in `loadConfig`).
- **Keep it dependency-light.** Stdlib plus `robfig/cron` and `yaml.v3` only.
  Don't pull in an HTTP or Discord SDK.
- **Discord facts worth remembering:** mentions only ping from `content`, never
  from an embed; limits are 25 fields/embed and 10 embeds/message — `discord.go`
  chunks for both. Preserve that when editing.
- **Runtime state** (`data/`) and the live `config.yaml` (holds webhooks) are
  gitignored; committed: source, `Dockerfile`, docs, and `config.example.yaml`.
- **Container runs as uid/gid 1000** and writes only `/app/data`. Keep it non-root.
- **Honkai Impact 3rd** has no public web-redemption page, so its codes post
  without a Redeem link (redeem in-game).
