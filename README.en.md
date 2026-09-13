# RoomDeck

For a fresh NAS LAN installation, use [compose.nas.yaml](compose.nas.yaml) and set your NAS IP. See the [NAS guide](docs/NAS.md) for storage, HTTPS and migration details.

**A shared space for people in the same room.**

[简体中文](README.md) · [Deployment](docs/DEPLOYMENT.md) · [Development](docs/DEVELOPMENT.md) · [Implementation status](docs/IMPLEMENTATION.md)

Now includes reactions, moderated live messages, reconnection and resumable uploads, a screen-sharing queue, and explicit poll selection for the display. See the [feature notes and limits](docs/FEATURES-2026-09.md) and [UI previews](docs/features-preview.html). Compose services and volume mappings remain unchanged.

Guests join a temporary shared space through a QR code or room code without an account. Share photos, files, notes and links; run polls; show selected content on a TV. Export the gathering and automatically remove content after its retention period.

[NAS 部署包 / NAS deployment bundle](https://github.com/Panda-995/RoomDeck/releases/download/nas-deploy-20260913/RoomDeck-NAS.zip) · [SHA256](https://github.com/Panda-995/RoomDeck/releases/download/nas-deploy-20260913/RoomDeck-NAS-SHA256SUMS.txt)

## Docker quick start

```sh
docker compose up -d
docker compose logs roomdeck
```

Open `http://YOUR-SERVER-IP:8080`. Use the **First-run setup token** from the logs to create your administrator. Setup is disabled after completion; guests never need this token. No separate database, image-processing service or hand-generated secret is required.

The default named volume persists application data; set `ROOMDECK_DATA_PATH=./data` to map it to a host directory. Compose starts the non-root RoomDeck application, LiveKit, and a one-time media-key initializer. Existing features remain available. No frontend CDN or external font is required.

Screen sharing needs trusted HTTPS, a reachable media IP and media ports. Optional HTTPS and TURN/TLS overlays are included; see [screen sharing deployment](docs/SCREEN-SHARING.md). Plain LAN HTTP supports files but does not generally permit screen capture.

For a domain/reverse proxy, copy `.env.example` to `.env`, set `BASE_URL` to an origin reachable by all participants, and rerun the startup command. LAN IP addresses work directly. Do not invite other devices using localhost.

## Included

Complete English/Simplified Chinese interfaces; secure initialization and host sign-in; guest nickname sessions; configurable rooms and invitations; JPEG/PNG/WebP thumbnails and previews; file downloads; notes and links; anonymous single/multiple-choice polls; WebSocket resynchronization; host moderation; read-only display pairing; slideshow/pause/blanking; ZIP exports with SHA-256 manifests; expiry cleanup and recovery.

## Boundaries

Also included: downloadable file-to-disk mappings, one-at-a-time publishing by any room member with multiple viewers and the paired display, plus [Undercover and Guess the dice](docs/GAMES.md) with private information and persistent game state. Mobile whole-screen publishing requires capabilities a web browser may not provide.

This is a single-instance development release: up to 100 rooms and 2,000 items per room. Default capacity is 2 GB and 50 participants per room, configurable in settings. Photos are limited to 20 MB/60 megapixels; files to 200 MB.

HEIC and videos can be shared as generic files; HEIC previews and video transcoding are not yet implemented. Original photos can contain metadata, so original downloads are off by default. Generated previews omit metadata. Anonymous polls deduplicate browser sessions, not real-world people. Public deployments should use HTTPS.

## Develop

Requires Go 1.27.1 and Node.js 22.22.2 or compatible versions:

```sh
cd web
npm ci
npm run build
cd ..
go run ./cmd/roomdeck
```

The app listens on port 8080 and uses `./data`. Run `go test ./...`, `go vet ./...`, `npm run build` and `npm run test:i18n`. Browser-test instructions are in the development guide. The old `prototype/` directory is a design artifact, not the production app.

The production interface uses warm white surfaces and a restrained green accent. Browse the [before-and-after UI preview](docs/ui-preview.html), [design system](DESIGN.md) and [UI/UX review and validation record](docs/UI-UX-AUDIT.md).


New: shared drawing board, timed Draw & guess, and scoped co-host permissions. See [Board and co-host guide](docs/BOARD-AND-COHOSTS.md).

## Published images and source builds

Use `ghcr.io/panda-995/roomdeck:latest` for Linux amd64 and arm64. Download `compose.yaml`, then run `docker compose pull` and `docker compose up -d`. Media initialization is bundled in the image. To build from source, use `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.
