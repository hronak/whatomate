# Whatomate

Modern, open-source WhatsApp Business Platform. Single binary app.

> **Note:** This is a fork of the original [shridarpatil/whatomate](https://github.com/shridarpatil/whatomate), not the upstream project. Releases and Docker images here are published under `hronak/whatomate`.
>
> **Caution:** This fork is under constant development and may break. Use at your own risk. Documentation may also be out of date.

## Differences from Upstream

This fork (`hronak/whatomate`) includes significant overhauls and security improvements over the original project since it was forked at commit `2e05458`:

- **Security & Stability:** Closed RBAC gaps with a declarative route table, improved concurrency and lifecycle management to prevent data loss, and secured embedded endpoints.
- **Backend Enhancements:** Upgraded to WhatsApp Graph API v26.0, replaced local Piper TTS with cloud AI providers (OpenAI, ElevenLabs, Google), and updated core dependencies (Go, PostgreSQL, Redis 8).
- **Frontend Overhaul:** Migrated to Tailwind v4, enforced modern typographic scales (minimum 1rem font size), and refreshed the AI chatbot integration (including Gemini 3.1 Pro and Gemini 3.7 Flash). Vue 3 idioms were heavily adopted, and unnecessary UI animations were removed for better performance.
- **DevOps & Tooling:** Dockerfiles were optimized for smaller image sizes, GitHub Pages documentation deployment was removed (along with the `docs/` directory), and the dev stack was consolidated to a single URL.


## Features

- **Multi-tenant Architecture**
  Support multiple organizations with isolated data and configurations.

- **Granular Roles & Permissions**
  Customizable roles with fine-grained permissions. Create custom roles, assign specific permissions per resource (users, contacts, templates, etc.), and control access at the action level (read, create, update, delete). Super admins can manage multiple organizations.

- **WhatsApp Cloud API Integration**
  Connect with Meta's WhatsApp Business API for messaging.

- **Real-time Chat**
  Live messaging with WebSocket support for instant communication.

- **Template Management**
  Create and manage message templates approved by Meta.

- **Bulk Campaigns**
  Send campaigns to multiple contacts with retry support for failed messages.

- **Chatbot Automation**
  Keyword-based auto-replies, conversation flows with branching logic, and AI-powered responses (OpenAI, Anthropic, Google).

- **Canned Responses**
  Pre-defined quick replies with slash commands (`/shortcut`) and dynamic placeholders.

- **Voice Calling & IVR**
  Incoming and outgoing WhatsApp calls with IVR menus, DTMF routing, call transfers to agent teams, hold music, and call recording.

- **Analytics Dashboard**
  Track messages, engagement, and campaign performance.



## Installation

### Docker

The latest image is available on Docker Hub at [`hronak/whatomate:latest`](https://hub.docker.com/r/hronak/whatomate)

```bash
# Download compose file, sample config, and env file
curl -LO https://raw.githubusercontent.com/hronak/whatomate/main/docker-compose.yml
curl -LO https://raw.githubusercontent.com/hronak/whatomate/main/config.example.toml
curl -L https://raw.githubusercontent.com/hronak/whatomate/main/.env.example -o .env

# Copy and edit config
cp config.example.toml config.toml
# Edit .env to set PostgreSQL credentials and timezone

# Run services
docker compose up -d
```

Go to `http://localhost:8080` and login with `admin@admin.com` / `admin`

__________________

### Binary

Download the [latest release](https://github.com/hronak/whatomate/releases) and extract the binary.

```bash
# Copy and edit config
cp config.example.toml config.toml

# Run with migrations
./whatomate server -migrate
```

Go to `http://localhost:8080` and login with `admin@admin.com` / `admin`

__________________

### Build from Source

```bash
git clone https://github.com/hronak/whatomate.git
cd whatomate

# Production build (single binary with embedded frontend)
make build-prod
./whatomate server -migrate
```



## Upgrading

Schema migrations are **applied by the app itself, not by a separate command** —
both Docker images default to `CMD ["server", "-migrate"]` and the shipped
`docker-compose.yml` passes `-migrate` explicitly, so a normal
`docker compose pull && docker compose up -d` migrates on boot. Every step is
idempotent. There is no manual migration step *unless* your deployment overrides
the container command (a Nomad/Kubernetes spec with a bare `server`) — then add
`-migrate` back, or migrations are silently skipped.

Two rules for any upgrade that crosses a schema change:

- **Back up PostgreSQL first.** Migrations drop columns, which is irreversible.
- **Migrate with one instance.** There is no advisory lock around
  `AutoMigrate`, so bring up a single `server -migrate` and scale extra app or
  `worker` containers out only after it is healthy.

### 0.2.0 → 0.3.5

```bash
pg_dump ... > whatomate-0.2.0.sql     # do this first
docker compose pull
docker compose up -d
docker compose logs -f app            # watch the migration run
```

`.env` needs no changes — its keys are unchanged since 0.2.0.

**`config.toml`: the `[tts]` section changed shape.** IVR greeting synthesis
moved from local Piper to cloud providers, so the old keys are gone:

| 0.2.0 (remove) | 0.3.5 |
| --- | --- |
| `piper_binary` | `provider = "openai"`, `"elevenlabs"` or `"google"` |
| `piper_model` | `openai_key`, `openai_voice` |
| `opusenc_binary` | `elevenlabs_key`, `elevenlabs_voice_id` |
| | `google_credentials_json`, `google_voice_name` |

This is the one edit an upgrade may require, and it **fails quietly**: unknown
TOML keys are ignored and an empty `provider` only logs a warning, so the server
starts normally with text-to-speech disabled. If your IVR flows use synthesized
greetings, configure a provider; if you don't use IVR, just delete the old keys.

Two keys are new, both with working defaults — nothing to add:

- `[server] read_buffer_size` (default 16384) caps the request line plus
  headers. fasthttp's old 4KB default was tight for cookie auth behind a reverse
  proxy, which surfaced as dropped connections logging `small read buffer`.
- `[app] frontend_dir` serves the SPA from disk instead of from the copy
  embedded in the binary. Leave it unset in production.

No other key changed name, type, or default, and none became required.
`enforce_route_permissions` already existed in 0.2.0, so route permissions stay
in shadow mode across the upgrade unless you flip it yourself.

**What the migration does.** 0.2.0 linked most rows to a WhatsApp account by
*name*; 0.3.x links by ID. On `-migrate` the app adds `whatsapp_account_id`,
resolves each legacy name against `whatsapp_accounts` within the owning
organization, then drops the name column. An empty name meant
"organization-wide" and is left NULL, which is how current queries spell the same
thing. Soft-deleted accounts still match. Don't run
`scripts/migrations/issue_16_whatsapp_account_fk.sql` — it is the hand-written
precursor, superseded by this migration.

A column is only dropped once **every** row resolved. If some name matches no
account, the migration aborts with `Migration failed` and names the table and row
count:

```
messages: 3 row(s) name a WhatsApp account that is not in whatsapp_accounts;
recreate the account or clear whats_app_account on those rows, then re-run the migration
```

Nothing is lost when that happens — the old column is still there and the
container just exits, so it will crash-loop until you either recreate the missing
account with its original name or clear that column on the affected rows. Then
restart; the migration picks up where it left off.

## CLI Usage

```bash
./whatomate install             # Set up the database, then exit
./whatomate server              # API + 1 worker (default)
./whatomate server -workers=0   # API only
./whatomate worker -workers=4   # Workers only (for scaling)
./whatomate version             # Show version
```

## Developers

The backend is written in Go ([Fastglue](https://github.com/zerodha/fastglue)) and the frontend is Vue.js 3 with shadcn-vue.
- If you are interested in contributing, please read [CONTRIBUTING.md](./CONTRIBUTING.md) first.

**Requirements:** Go 1.26, Node 24, and Docker (for Postgres and Redis).

```bash
git clone https://github.com/hronak/whatomate.git
cd whatomate

make dev-infra   # Postgres + Redis in Docker
make install     # Create the schema and the default admin
make dev         # Backend + frontend
```

Open <http://localhost:3000> and log in with `admin@admin.com` / `admin`.

There is nothing to copy and nothing to edit first: development targets use
[`dev/config.toml`](dev/config.toml), which is checked in and contains no
secrets. For real credentials copy `config.example.toml` to `config.toml`
(gitignored) and pass `make run CONFIG=config.toml`.

`make install` is idempotent, so it is safe to re-run after a `git pull`. Add
`make seed` for demo contacts, tags, and a starter chatbot flow to look at.

### The two ports

| | Serves | Use when |
| --- | --- | --- |
| **:3000** Vite | the frontend from live source, `/api` and `/ws` proxied to :8080 | always, while developing |
| **:8080** Go | the API, plus the frontend from `frontend/dist` | verifying a build |

In development `app.frontend_dir` points the backend at `frontend/dist`, so
`make frontend-build` alone refreshes :8080 — no Go rebuild, no restart. Shipped
binaries leave that unset and serve the frontend embedded by `make build-prod`.

### Everything in containers

If you would rather not install Go and Node locally:

```bash
make dev-docker    # backend + Vite + Postgres + Redis, all containerized
make rm-dev-docker # tear it down, including the database volume
```

VS Code users can instead run **Dev Containers: Reopen in Container** — the
[`.devcontainer`](.devcontainer) config brings up the same stack and runs
`make install` for you.

> The dev stack publishes Postgres on 5432 and Redis on 6379, the same ports as
> the production `docker-compose.yml`. Don't run both at once.

### Other targets

`make help` lists them all. The common ones are `make test`, `make lint`,
`make build-prod`, and `make test-e2e`.

## License

See [LICENSE](LICENSE) for details.
