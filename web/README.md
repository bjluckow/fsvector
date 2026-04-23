# TypeScript/React/Vite/Express Boilerplate

Vite + React + TypeScript client with an Express + TypeScript server, wired up as an npm workspace.

## Setup

```bash
./scripts/rename.sh myproject
npm install
npm run dev
```

Client runs on `localhost:5173`, server on `localhost:3001`. The Vite dev server proxies `/api` to the server. Set `VITE_API_URL` in `.env` to override.