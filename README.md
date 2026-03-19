# Vibe PoGo Appraisal App

Vibe PoGo Appraisal App is a full-stack tool that analyzes Pokemon GO appraisal screenshots/videos and turns them into structured results (species, CP, HP, IVs, level, and source metadata).

Production website: https://vibepogo.victoroliveira.com.br

## Human Generated

I created this project to experiment with some vibecode workflows. Don't take it too seriously.
My only commitment is to not write any code in this repo, not even the rest of this document is written by me! 🤖❌⚠️✅🚀

> I am absolutely right!

### Problem Statement

I don't want to pay for PokeGenie. I want to be able to quickly parse my Pokemon to select the best for PvP battles.
My current way was 1. manually screenshotting my Pokemon 2. opening PokeGenie 3. reading screenshot 4. memorizing best candidates

My ideal workflow is: 1. screen record my phone in Pokemon GO as I go through my list of potential PvP 2. feed the whole video to the system 3. let the system parse the images from the video and extract the different Pokemon 4. rank them automatically for me + consolidate my list of PvP Pokemon

### Goals

1. Experiment vibe code workflows
2. Offer me a better experience of selecting my PvP teams

### Roadmap

- [ ] Make pending Pokemon appear inline with the rest of Pokemon
- [ ] Add proper login
- [ ] Add Pokemon Detailed view - Display level, stardust required to level up by league etc.
- [ ] Add Team Builder for League view
- [ ] Implement worker pool
- [ ] Use a proper queue instead of sqlite

## What It Does

- Accepts screenshot or video uploads from the browser UI.
- Creates an anonymous session and processes uploads asynchronously.
- Runs OCR + appraisal parsing in a background worker.
- Stores and serves parsed Pokemon results through the API.
- Supports pending species confirmation when OCR needs user disambiguation.
- Supports retrying failed jobs from the UI.

## Architecture

- `frontend/`: React + Vite SPA (`/` upload/results flow, `/healthz`).
- `web/`: Go API service (`/session`, `/uploads`, `/jobs/{jobId}`, `/jobs/{jobId}/retry`, `/pokemon`, `/pokemon/pending-species`).
- `worker/`: Go background processor (queue polling, image/video processing, OCR, species catalog matching).
- `docker-compose.yml`: Local orchestration for `frontend`, `web`, and `worker`.
- SQLite/libSQL-backed persistence with local fallback path (`./var/app.db`) in local mode.

## Main Features

- Image and video upload flow.
- Upload status panel with lifecycle/progress and retry actions.
- Background job queue with worker health checks.
- Pokemon results panel (including IVs and extracted metadata).
- Pending-species resolution flow for ambiguous OCR outcomes.
- Local deterministic media storage mode and production UploadThing mode.

## Run Locally

### Prerequisites

- Docker + Docker Compose
- Node.js + npm
- Go 1.22+

### 1) Bootstrap

```bash
make bootstrap
```

This will:

- Create `.env` from `.env.example` if missing.
- Create local writable directories/files (`var/`, `testdata/uploads`, `var/app.db`).
- Install frontend deps and download Go modules.

### 2) Start The Stack

```bash
make up
```

`make up` starts all services in Docker and runs smoke tests.

If you want to start without running smoke tests:

```bash
make dev
```

### 3) Access Services

- Frontend: `http://localhost:4173`
- API health: `http://localhost:8080/healthz`
- Worker health: `http://localhost:8081/healthz`

### 4) Stop / Logs

```bash
make logs
make down
```

## Environment Configuration

Use `.env.example` as your base. Key variables:

- `APP_ENV=local`
- `WEB_PORT=8080`
- `FRONTEND_PORT=4173`
- `WORKER_HEALTH_PORT=8081`
- `DATABASE_PATH=./var/app.db` (local fallback)
- `UPLOAD_STORAGE_MODE=local` for local development
- `UPLOAD_LOCAL_DIR=./testdata/uploads`

For production/deploy-style runs, use `.env.production.example` as reference (`UPLOAD_STORAGE_MODE=uploadthing`, remote DB URL/token, CORS origin, etc.).

## Useful Commands

```bash
make bootstrap      # initial local setup
make up             # start stack + smoke tests
make dev            # start stack only
make test-smoke     # run smoke checks manually
make logs           # tail compose logs
make down           # stop local stack
make prod-up        # start production compose with .env.production
make prod-down      # stop production compose
```
