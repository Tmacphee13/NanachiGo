# Nanachi (in Go)

## Research Paper MindMap Generator

Take in and parse pdf files, call LLM to extract main points and generate summaries, present in spoke and wheel graph with external nodes being more granular subcategories. Taken with heavy inspiration from John Damask.


## Multi-Platform Support (AWS and GCP)

This Go backend can run against either:
- AWS: Bedrock (Claude) + DynamoDB
- GCP: Gemini + Firestore

You can switch platforms from the UI (Source: AWS | GCP) or via the `platform` query param on API calls. If no platform is specified, the server uses `DEFAULT_PLATFORM`.

Frontend behavior
- The library and admin pages include a “Source” toggle that stores the selection in `localStorage` and appends `?platform=aws|gcp` to API requests.
- List, upload, delete and node edit actions are all platform-aware.

Environment variables
- Shared
  - `ADMIN_PASSWORD` – admin login password (defaults to `admin` if not set)
  - `DEFAULT_PLATFORM` – `aws` or `gcp` (defaults to `aws`)
- AWS
  - `AWS_REGION`
  - Standard AWS credentials in environment (and optional session token)
  - `MINDMAPS_TABLE` – DynamoDB table name (defaults to `mindmaps`)
- GCP
  - `GCP_PROJECT_ID`
  - `GOOGLE_APPLICATION_CREDENTIALS` – path to a service account JSON with Firestore access
  - `GEMINI_API_KEY` – API key for Gemini

Firestore configuration
- Firestore collection defaults to `mindmaps`.

Run (Go backend)
- Ensure your `.env` has the variables you need (the server loads `.env` at startup).
- Start the Go server:
  - `go run cmd/server/main.go`
  - Server listens on `http://localhost:3000`

API overview
- `GET /api/mindmaps?platform=aws|gcp` – list mind maps from DynamoDB or Firestore
- `POST /api/upload?platform=aws|gcp` – upload a PDF, extract metadata + mind map via Bedrock/Gemini, persist to DB
- `DELETE /api/mindmaps/:id?platform=aws|gcp` – delete a mind map by ID
- `POST /api/mindmaps/:id/redo-description?platform=aws|gcp` – regenerate a node’s tooltip
- `POST /api/mindmaps/:id/remake-subtree?platform=aws|gcp` – rebuild a node’s children
- `POST /api/mindmaps/:id/go-deeper?platform=aws|gcp` – add a deeper level from a leaf

Notes
- The legacy Node server (`server.js`) remains in the repo for reference but the Go server is the primary path.
- Timestamps are stored as ISO strings in Go; the frontend handles both ISO and Firestore timestamp objects.

