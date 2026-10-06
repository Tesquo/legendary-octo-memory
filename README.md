# legendary-octo-memory

A local media player with a Go backend and a React frontend that runs in the
browser. The Go layer turns arbitrary local files into something a browser can
play, which is the hard part it exists to solve.

## Architecture

- **Go backend (`:8080`)** — runs on the host machine only. Classifies local
  files, runs cancellable ffmpeg jobs on a goroutine worker pool, reports
  progress over Server-Sent Events, and serves playable video with HTTP range
  support so seeking is instant.
- **React frontend** — room-first: selecting media opens a theatre with a share
  sidebar. It talks to the Go backend on `localhost` and is built around two
  seams so it stays server-ready:
  - `MediaSession` (`web/src/lib/MediaSession.js`) abstracts the thing being
    played, so a WebRTC or server-backed session can replace the local one
    without touching the UI.
  - The API client (`web/src/lib/api.js`) reads `VITE_API_BASE`, so the same
    build can point at a deployed backend.

## How preparation works

Each upload is inspected with `ffprobe` and placed into one of three buckets
(`internal/processing/classify.go`):

| Strategy    | When                                          | What happens                                        |
| ----------- | --------------------------------------------- | --------------------------------------------------- |
| `playable`  | Container and codecs are browser-friendly     | Served as-is; marked `ready` immediately            |
| `remux`     | Codecs fine, container unsupported (e.g. MKV) | Streams copied into `playable.mp4` (fast, lossless) |
| `transcode` | Codec incompatible (e.g. HEVC)                | Re-encoded to H.264/AAC, capped at 1920px wide      |

Jobs run on a bounded worker pool (`internal/processing/pipeline.go`), each in a
cancellable context, with a goroutine tailing ffmpeg's stderr to emit a
percentage. Progress fans out through `internal/ws/progress.go`.

A media item holds **at most one job at a time**, and cancelling is real:
deleting an item or re-processing it stops the running job and waits for ffmpeg to
exit (`CancelAndWait`) before the handler touches the item's files, so a delete
cannot be undone by a still-running job recreating `media/<id>/`.

## Running it

Backend (from the repo root):

```sh
go run ./cmd/server
```

Frontend:

```sh
cd web
npm install
npm run dev
```

Then open the Vite URL and select a video. Choosing a file opens a room straight
away, so the share link exists while the file is still being prepared; a **Start
watching** button appears once it is ready, and playback starts when you press it.

> On Windows PowerShell, if `npm` is blocked by the execution policy, use
> `npm.cmd` instead.

## API

| Method | Path                | Purpose                                     |
| ------ | ------------------- | ------------------------------------------- |
| GET    | `/health`           | Health check                                |
| GET    | `/media`            | List the library                            |
| GET    | `/media/{id}`       | Single item detail                          |
| DELETE | `/media/{id}`       | Delete an item (row + files)                |
| POST   | `/upload`           | Upload a file (multipart `file`)            |
| POST   | `/reprocess/{id}`   | Re-run preparation for an item              |
| GET    | `/play/{id}`        | Stream the playable rendition (range-aware) |
| GET    | `/open/{id}`        | Raw source file (no UI caller today)        |
| GET    | `/progress`         | SSE stream of pipeline progress             |
| GET    | `/media-file/*`     | Static thumbnails and renditions            |

## Configuration

Settings live in `config/config.go` and are read from the environment at
startup, with local-development defaults:

| Env var            | Default                 | Meaning                      |
| ------------------ | ----------------------- | ---------------------------- |
| `HOST`             | `127.0.0.1`             | Interface the API binds to   |
| `PORT`             | `8080`                  | HTTP listen port             |
| `WORKER_COUNT`     | `2`                     | Concurrent ffmpeg jobs       |
| `MAX_UPLOAD_BYTES` | `8589934592` (8 GiB)    | Max size of a single upload  |
| `ALLOWED_ORIGINS`  | `http://localhost:5173`, `https://tesquo.github.io` | Comma-separated allowed origins |

`HOST` defaults to loopback on purpose. The API is unauthenticated and by design
only the host's own browser calls it, so out of the box nothing else on the
network can list, upload to, or delete from the library. Set `HOST=0.0.0.0` only
if you intend to serve the API to other machines.

Requests are also checked server-side before they reach a handler
(`internal/api/guard.go`): while `HOST` is loopback (the default) the `Host`
header must name loopback too, and a state-changing request (`POST`, `PUT`,
`DELETE`) must carry an `Origin` from the allow-list. CORS alone cannot do this
job, because the CORS middleware does not
reject a disallowed origin — it runs the handler anyway and merely omits the
`Access-Control-Allow-*` headers, so only the caller's JavaScript is kept from
reading the reply. The allow-list decides who may *call* the API, and the host
check is what stops a domain that re-resolves to `127.0.0.1` from being treated
as same-origin. Two deliberate omissions: a state-changing request with no
`Origin` at all is allowed (that is what `curl` and scripts look like), and a UI
served by this binary itself needs no entry, since a same-origin request always
matches whatever `PORT` is set to. The `Host` check applies while `HOST` is
loopback (the default); once the API is bound to a routable address a rebound
page's `Origin` and `Host` agree with each other, so nothing here can tell it
from a legitimate client — treat that mode as exposed, and put a per-request
secret in front of it if you need one.

The frontend has its own build-time settings (`web/src/lib/config.js`), read from
Vite env vars in `web/.env.local`:

| Env var                | Default                  | Meaning                              |
| ---------------------- | ------------------------ | ------------------------------------ |
| `VITE_API_BASE`        | `http://localhost:8080`  | Where the Go API lives               |
| `VITE_DEFAULT_VOLUME`  | `0.3`                    | Initial playback volume              |
| `VITE_AUTOPLAY`        | unset (off)              | `true` to attempt playback on open   |
| `VITE_ICE_SERVERS`     | public STUN              | JSON array of `RTCIceServer` objects |

## Sharing (peer-to-peer)

Choosing a video opens a room immediately: the invite link appears in the room's
sidebar while the file is still being prepared, so it can be sent out before
anyone presses play. The link looks like:

```
http://localhost:5173/#/watch/<roomId>
```

Open that link in another tab (or send it to someone) and press **Watch**. The
host's video is forwarded as a live WebRTC stream — viewers see exactly what the
host plays, so they stay in sync automatically.

How it works:

- **Signaling** uses PeerJS's free public broker, so there is no server to run.
  Both sides ask PeerJS to reconnect if that socket drops: the peer-to-peer
  connections (and any media already flowing) survive it, so the reconnect stays
  silent rather than being reported as a connection problem.
- **Media** flows directly browser-to-browser and never touches the backend.
- The **viewer** opens a data channel first; that reveals their peer id to the
  host, who then calls them with the stream. The viewer's page never calls the
  Go API — on their machine `localhost` would be their own computer.
- The host captures its own `<video>` with `captureStream()`, so the share is a
  live re-encode rather than a file transfer.

### The playlist

The room's sidebar holds the playlist, and it is the only view of the library a
viewer ever gets. The host owns it outright: playing an item puts it on the
playlist, letting one finish advances to the next, and more can be added from a
new upload or from the rest of the library with **Add media**.

A viewer sees the same list read-only, plus a line describing the connection, and
follows automatically because it is watching the host's live stream rather than
its own copy. What a viewer is told — and nothing else from the library — travels
over the data channel as a single versioned message (`web/src/lib/protocol.js`),
replayed to each viewer as it connects.

Moving to another item hangs up each viewer and calls them again, since PeerJS
cannot swap the media on a live call: expect a brief re-buffer on the switch.

Things to be aware of:

- The **host's tab must stay open** while sharing — leaving the room ends the
  session for everyone watching.
- Home connections give much less **upload** than download, and each viewer costs
  a full copy of the stream. A handful of viewers at 1080p is realistic.
- PeerJS's public broker provides **signaling only**; the connection itself is
  negotiated with the ICE servers in `web/src/lib/config.js`. STUN is the default
  and TURN is opt-in, so a minority of network pairs (symmetric NAT, some mobile)
  keep failing to connect until a relay is configured — see § "TURN relay".
- A viewer on a **different machine** needs the app itself to be reachable, so the
  frontend has to be deployed somewhere public — see § "Deploying the frontend
  (static site)".

### TURN relay

STUN alone is enough on a LAN and on most home networks, but the peer pairs that
cannot be reached directly — symmetric NAT, strict corporate or mobile firewalls —
need a **TURN** server to relay the media. Roughly 10-20% of pairs fall into that
bucket, and without TURN those viewers simply fail to connect.

TURN is a server, so it is configured rather than assumed. Point the frontend at
one with `VITE_ICE_SERVERS`, a JSON array of `RTCIceServer` objects:

```sh
# web/.env.local
VITE_ICE_SERVERS=[{"urls":"stun:stun.l.google.com:19302"},{"urls":"turn:turn.example.com:3478","username":"user","credential":"pass"}]
```

- **Open Relay Project** (`openrelay.metered.ca`, free, no signup) is the quickest
  way to try a relay: `turn:openrelay.metered.ca:80` with `openrelayproject` /
  `openrelayproject`.
- **Self-hosted coturn** is the durable option if you run your own server.
- **Cloudflare Calls** and **Twilio** sell TURN as a service with usage-based
  pricing.

Leaving `VITE_ICE_SERVERS` unset keeps the built-in public-STUN default, which is
fine locally. Nothing in the UI changes when a relay is added — it is the same
peer connection, just more likely to succeed.

## Deploying the frontend (static site)

The frontend is plain static files (`web/dist/`), so any static host can serve it.
This is what makes a share link usable on another machine: a `localhost` link would
point at the *viewer's* computer, which has no app and no backend.

The thing to hold on to: **only the frontend moves.** The Go backend stays on the
host's machine, bound to loopback, because that is where the files live. A viewer
never calls it (§ "Sharing"), so one build works for everyone — the host's browser
reaches its own `localhost:8080`, and a viewer's browser talks only over WebRTC.

**1. Build**

```sh
cd web
npm run build        # -> web/dist/
```

**2. Host it.** The app uses a hash router, so **no SPA rewrite/redirect rules are
needed** — every host just serves `index.html` and the hashed assets.

| Host             | What to do                                                             |
| ---------------- | ---------------------------------------------------------------------- |
| Netlify          | Commit `netlify.toml` (already set to `web/`), then connect the repo or run `netlify deploy --prod` |
| Vercel           | New project → set **Root Directory** to `web` (Vite is auto-detected)   |
| Cloudflare Pages | Build command `npm run build`, output `dist`, root `web`                |
| GitHub Pages     | Push to `main` builds `web/` and publishes `web/dist` via `.github/workflows/deploy-pages.yml`; enable it once under **Settings → Pages → Source = GitHub Actions** |
| Quick local/LAN  | `cd web && npx serve dist`, then open the LAN IP                        |

> **GitHub Pages serves from a sub-path.** A project site lives at
> `https://<owner>.github.io/<repo>/`, so the build needs a matching Vite base —
> that is why the workflow sets `VITE_BASE=/<repo>/` (use `/` for a custom domain
> or a `<owner>.github.io` user site). Note that `ALLOWED_ORIGINS` wants the
> **origin only**, with no repo path: `https://<owner>.github.io`. A Pages site
> is reachable at that URL by anyone, even if the repository is private.

**3. Let the host's browser call its own backend.** The host still runs the Go
server locally, but its browser now loads the page from the deployed origin and
calls `http://localhost:8080` cross-origin. Allow that origin (comma-separated;
the CORS middleware already covers every route, including `/progress`):

```sh
# macOS / Linux / Git Bash
ALLOWED_ORIGINS=https://your-app.netlify.app go run ./cmd/server
```

```powershell
# Windows PowerShell
$env:ALLOWED_ORIGINS = "https://your-app.netlify.app"; go run ./cmd/server
```

```bat
:: Windows cmd.exe
set ALLOWED_ORIGINS=https://your-app.netlify.app && go run ./cmd/server
```

The value is read **once at startup** (`config/config.go`) and is
**comma-separated**. It must match the browser's `Origin` header exactly — scheme
+ host + port, no trailing slash, no path. Setting the variable replaces the
defaults, which already include `http://localhost:5173` (the dev server) and this
project's Pages origin, so add those back if you override it.

The list is enforced server-side as well as honoured by the browser
(`internal/api/guard.go`), so a call from an unlisted origin is refused outright
rather than merely hidden from the caller's JavaScript.

`VITE_API_BASE` stays `http://localhost:8080` — the host's own machine. A viewer
never calls the API, so the value is irrelevant on their machine.

> A deployed page is served over `https`, and browsers treat `http://localhost` as
> a trustworthy origin, so the call above is allowed. If a browser blocks it, serve
> the app over plain `http` for that pass.

**4. Optional extras.** Add a **TURN** relay (`VITE_ICE_SERVERS` in the build
environment, § "TURN relay") for viewer pairs STUN alone cannot connect. The
default **signalling** broker is PeerJS's public one, which works whenever both
peers have internet; self-host it only for LAN/offline use.

See `web/.env.example` for every build-time setting.

## Roadmap

Sharing is implemented for the local/two-tab case, the frontend has a documented
static deployment path for cross-machine links, and the host-owned playlist is in
place (append/remove, auto-advance, an editable list for the host and the same list
read-only for viewers). The reliability pass is done: delete and re-process can no
longer race a running ffmpeg job, and a signalling blip reconnects instead of
ending a room. Likely next steps: reorderable and persisted playlists, configuring
a TURN relay in production, seamless item switching (a switch currently
re-buffers), and an optional server mode where a media server (SFU) replaces the
peer mesh so the host's upload does not scale with viewer count.

## Development notes

`.agents/skills/legendary-octo-memory/SKILL.md` is the working reference for this
codebase: architecture, the design seams, conventions, exact commands, and a
list of pitfalls already hit (stale server processes, static-file URL prefixing,
the React Compiler lint rules, and more). Read it before making changes —
`SKILL.md` at the repo root is a short pointer to it.

`.agents/skills/p2p-watch-party-deploy/SKILL.md` is a portable, host-agnostic
recipe for taking a peer-to-peer watch-party app from `localhost` to a shareable
link (static hosting, signaling, TURN, CORS). Reach for it when deploying this app
or any other codebase with the same shape.

