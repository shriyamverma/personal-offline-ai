# Shriyam AI

A personal chat assistant with a React frontend, a Go API, Groq chat generation, and Qdrant Cloud semantic memory. Conversations and summaries are stored in SQLite on your server.

The current application uses cloud services and requires an internet connection. Docker runs the frontend and API; model inference and embeddings run remotely.

## Versions

The project maintains two permanent branches:

| Branch              | Version                         | Model and memory services                                      |
| ------------------- | ------------------------------- | -------------------------------------------------------------- |
| `main`              | Offline assistant               | Local inference and embeddings through Ollama                  |
| `personal-cloud-ai` | Cloud assistant documented here | Groq chat generation and Qdrant Cloud embeddings and retrieval |

Cloud changes target `personal-cloud-ai`. Offline changes target `main`; shared fixes can be cherry-picked between the permanent branches.

## Features

- **Streaming chat:** responses arrive as they are generated, with Markdown rendering and copyable code blocks. Stop an in-progress response from the composer.
- **Conversation management:** create, reopen, rename, delete, and search conversation titles.
- **Model selection:** choose a Groq model per conversation and a default model for new chats. Refresh available models in Settings.
- **Persistent history:** messages, summaries, and request status survive browser refreshes and container restarts.
- **Conversation memory:** recent messages, automatic summaries, and relevant messages retrieved through Qdrant provide context for replies. Retrieval is scoped to the current conversation and database installation.
- **Memory recovery:** if Qdrant becomes unavailable after startup, chat falls back to recent history and saved summaries, displays a warning, and retries indexing on later requests.
- **Appearance settings:** light, dark, or system appearance, with independent palettes for each mode. Available palettes are Forest, Midnight Indigo, Warm Graphite, Paper & Ink, Nordic Blue, and Monochrome.
- **Responsive interface:** desktop and mobile layouts, browser-saved preferences, and configurable Enter-to-send behavior.

## Requirements

- Docker with Docker Compose.
- A Groq API key with access to a supported chat model.
- A Qdrant Cloud cluster with Cloud Inference enabled and a read-write cluster data API key.

## Quick Start

Run these commands from the repository root.

### 1. Configure the services

```bash
cp .env.example .env
```

Fill in these values in `.env`:

```dotenv
GROQ_API_KEY=your-groq-api-key
QDRANT_URL=https://your-cluster-endpoint
QDRANT_API_KEY=your-qdrant-cluster-data-api-key
```

Use the Qdrant cluster endpoint and its **data API key**, rather than an account-management token. Enable Cloud Inference in the cluster settings.

Keep `.env` private. It is ignored by Git, and credentials are passed only to the backend container.

### 2. Start the application

```bash
docker compose up -d --build
```

Open [http://localhost:3000](http://localhost:3000).

The API validates provider configuration and initializes the Qdrant collection at startup. Both services must be configured before it can start successfully.

### 3. Check the services

```bash
docker compose ps
curl -fsS http://localhost:3000/api/
curl -fsS http://localhost:3000/api/models
```

The health response should contain `"status":"ok"`. The models response lists configured chat models available through your Groq account.

To inspect startup errors:

```bash
docker compose logs --tail=100 assistant-api
```

## Services

| Service       | URL                                            | Purpose        |
| ------------- | ---------------------------------------------- | -------------- |
| Shriyam AI    | [http://localhost:3000](http://localhost:3000) | Chat interface |
| Assistant API | [http://localhost:8000](http://localhost:8000) | Go backend     |

The frontend proxies `/api/` requests to the backend. Compose binds both published ports to `127.0.0.1`.

## Configuration

| Variable            | Default / purpose                                                                                |
| ------------------- | ------------------------------------------------------------------------------------------------ |
| `GROQ_API_KEY`      | Required Groq API key                                                                            |
| `GROQ_BASE_URL`     | `https://api.groq.com/openai/v1`                                                                 |
| `CHAT_MODEL`        | `openai/gpt-oss-20b`; backend default for new conversations                                      |
| `GROQ_CHAT_MODELS`  | `openai/gpt-oss-20b,openai/gpt-oss-120b`; model allowlist, filtered against Groq's active models |
| `QDRANT_URL`        | Required Qdrant cluster endpoint                                                                 |
| `QDRANT_API_KEY`    | Required cluster data API key                                                                    |
| `QDRANT_COLLECTION` | `shriyam_ai_memory_minilm_v1`                                                                    |
| `DATABASE_PATH`     | `/data/assistant.db` in Docker; `./assistant.db` when running Go directly                        |

The embedding model is `sentence-transformers/all-minilm-l6-v2`, with 384-dimensional vectors and Cosine distance. The API creates the collection and scope indexes if needed. Use a dedicated collection; an existing collection must match this configuration and embedding model.

The Compose file sets `DATABASE_PATH` directly. Change that service setting and its storage mount together if you need a different Docker database location.

## History, Memory, and Privacy

SQLite is the source of truth for messages and summaries. Docker persists it in the `assistant_cloud_data` named volume. Qdrant holds embeddings and identifiers used to retrieve messages from the correct SQLite conversation.

Prompts and retrieved context are sent to Groq. Message text and search queries are sent to Qdrant for embedding and retrieval. Qdrant payloads contain identifiers rather than duplicate message bodies. Browser preferences are stored locally in the browser; chat history is stored on the server.

Deleting a conversation removes its Qdrant points before deleting its local history. If remote cleanup fails, the conversation is retained so deletion can be retried.

Existing databases are migrated without discarding messages or summaries. Requests left active by a previous process, including pending operations from older versions, become interrupted at startup so conversations can continue.

## Everyday Commands

```bash
# Start existing containers
docker compose up -d

# Rebuild after code or configuration changes
docker compose up -d --build

# Follow logs
docker compose logs -f

# Stop containers while retaining saved history
docker compose down
```

## Local Development

For development outside Docker, install Go 1.24.4 or compatible, a C compiler for the SQLite driver, and Node.js 24 with npm.

Start the backend in one terminal using your configured `.env`:

```bash
cd assistant-api
set -a
source ../.env
set +a
go run .
```

Start the frontend in a second terminal:

```bash
cd frontend
npm ci
npm run dev
```

Open [http://localhost:5173](http://localhost:5173). Vite proxies `/api` to `http://127.0.0.1:8000`; set `API_PROXY_TARGET` when using a different backend address.

Native development uses its own SQLite file by default, separate from the Docker volume.

## Validation

Backend tests use mock Groq and Qdrant servers and do not require provider credentials:

```bash
cd assistant-api
go test -race ./...
go vet ./...
```

Frontend unit tests, production build, and browser tests:

```bash
cd frontend
npm ci
npm test
npm run build
npx playwright install --with-deps chrome
npm run test:e2e
```

For a manual check, send a message, refresh to verify history, switch models, rename a conversation, and change appearance settings. Save a phrase such as `Remember this test phrase: copper mango lighthouse.`, then ask for it in the same conversation. Open a separate conversation to check that the phrase is not retrieved from the first chat.

## Project Structure

```text
assistant-api/
  main.go                  Startup, configuration, and shared API types
  handlers.go              Health, chat, conversations, and model endpoints
  run.go                   Streaming responses and conversation summaries
  cloud_memory.go          Semantic retrieval and indexing retries
  internal/llm/            Groq client
  internal/memory/         SQLite persistence and migrations
  internal/vectors/        Qdrant client
frontend/
  src/                     React interface, themes, and API client
  e2e/                     Browser regression tests
docker-compose.yml         Frontend, API, and persistent SQLite volume
.env.example               Backend configuration template
```
