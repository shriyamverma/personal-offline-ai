# Personal Offline AI Assistant

A self-hosted, lightweight personal AI assistant designed to run entirely on local systems with a strong focus on privacy, offline capability, and developer extensibility.

This project is focused on learning and building real-world AI assistant infrastructure from scratch, including:

- local LLM orchestration
- persistent memory
- semantic retrieval
- AI backend engineering
- containerized deployment
- retrieval-augmented generation (RAG)

---

# Features

## Current Features

- Fully offline AI assistant
- Local LLM inference using Ollama
- Open WebUI integration
- Go-based backend API
- Persistent SQLite memory
- Conversation history
- Semantic memory retrieval using embeddings
- Dockerized architecture
- Makefile-based developer workflow
- Multi-step AI agent with autonomous tool planning
- Hybrid tool routing (rule-based routing + LLM-based planning)
- Workspace-aware file search and inspection
- Controlled file creation with explicit approval
- Restricted shell command execution with explicit approval
- Git integration (status and diff)
- Sequential multi-tool execution (up to 4 steps per request)
- Semantic retrieval using Ollama embeddings
- Conversation summarization for long-term context
- Server-Sent Events (SSE) for agent execution progress

---

# Architecture

```text
User
  ↓
Open WebUI / API Client
  ↓
Assistant API (Go)
  ↓
Agent Orchestrator
  ├── Hybrid Router
  ├── LLM Planner
  ├── Tool Registry
  │     ├── File Tools
  │     ├── Git Tools
  │     └── Shell Tools
  │
  ├── Memory Layer
  │     ├── SQLite
  │     ├── Conversation History
  │     ├── Embeddings
  │     └── Semantic Retrieval
  │
  ↓
Ollama
  ↓
Qwen2.5:3B
```

---

# Tech Stack

| Layer            | Technology     |
| ---------------- | -------------- |
| LLM Runtime      | Ollama         |
| Model            | Qwen2.5:3B     |
| Backend          | Go             |
| UI               | Open WebUI     |
| Database         | SQLite         |
| Containerization | Docker         |
| Orchestration    | Docker Compose |

---

# Project Structure

```text
personal-offline-ai/
│
├── assistant-api/
│   ├── internal/
│   │   ├── agent/
│   │   │   ├── planner.go
│   │   │   ├── router.go
│   │   │   └── router_test.go
│   │   │
│   │   ├── memory/
│   │   │   ├── db.go
│   │   │   └── models.go
│   │   │
│   │   └── tools/
│   │       ├── types.go
│   │       ├── registry.go
│   │       ├── tool.go
│   │       ├── path.go
│   │       ├── read_file.go
│   │       ├── list_files.go
│   │       ├── search_files.go
│   │       ├── write_file.go
│   │       ├── shell.go
│   │       ├── git.go
│   │       └── shell_test.go
│   │
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   └── main.go
│
├── ollama/
│   ├── Dockerfile
│   └── bootstrap.sh
│
├── open-webui/
│   └── Dockerfile
│
├── docker-compose.yml
├── Makefile
└── README.md
```

---

# Getting Started

## Prerequisites

Install:

- Docker Desktop
- Git (for managing the project repository)
- Go (optional for local development; available inside the assistant-api container)

---

# Setup

## 1. Clone Repository

```bash
git clone https://github.com/shriyamverma/personal-offline-ai
cd personal-offline-ai
```

---

## 2. Start Services

```bash
make up
```

or

```bash
docker compose up --build -d
```

---

## 3. Pull Model

```bash
docker exec -it ollama ollama pull qwen2.5:3b
```

---

# Services

| Service       | URL                                              |
| ------------- | ------------------------------------------------ |
| Assistant API | [http://localhost:8000](http://localhost:8000)   |
| Open WebUI    | [http://localhost:3003](http://localhost:3003)   |
| Ollama API    | [http://localhost:11434](http://localhost:11434) |

---

# API Usage

## Health Check

```bash
curl http://localhost:8000/
```

Expected response:

```json
{
  "status": "ok",
  "service": "assistant-api"
}
```

---

## Chat Endpoint

```bash
curl -X POST http://localhost:8000/chat \
-H "Content-Type: application/json" \
-d '{"prompt":"Explain Docker simply"}'
```

Example response:

```json
{
  "response": "Docker is a containerization platform..."
}
```

## Multi-step Agent Execution

The agent can inspect the workspace, execute multiple tools, and use their outputs to generate a final response.

Example:

```bash
curl -X POST http://localhost:8000/chat \
-H "Content-Type: application/json" \
-d '{
  "prompt": "Inspect the project in the assistant-api. Start by listing files in \".\". Then inspect the Go entry point and important packages. Explain the architecture, major components, and how requests flow through the application."
}'
```

## Streaming Agent Progress

The API also supports Server-Sent Events (SSE) for receiving agent execution progress.

```bash
curl -N -X POST http://localhost:8000/chat \
-H "Content-Type: application/json" \
-d '{
  "prompt": "Inspect the project and summarize its architecture",
  "stream": true
}'
```

The stream provides events for:

- Tool execution started
- Tool execution result
- Tool execution errors
- Final agent response

Note: This currently streams agent execution events and the final response, not individual LLM-generated tokens.

---

# Memory System

The assistant uses SQLite as its persistent memory store.

## Current capabilities

- Stores user messages and assistant responses.
- Maintains conversation history.
- Retrieves recent messages to provide conversational context.
- Generates embeddings using Ollama's embedding API.
- Performs semantic retrieval using embedding similarity.
- Retrieves relevant past conversations based on the current query.
- Generates conversation summaries periodically to maintain context across longer conversations.

## Architecture

User Query
↓
Embedding Generation (Ollama)
↓
SQLite Memory Store
↓
Semantic Similarity Search
↓
Relevant Context
↓
LLM Response Generation

---

# Tool System

The assistant includes a pluggable tool execution framework that allows the AI agent to interact with the local development workspace.

## Available Tools

| Tool           | Description                         | Approval Required |
| -------------- | ----------------------------------- | ----------------- |
| `read_file`    | Reads files from the workspace      | No                |
| `list_files`   | Lists files and directories         | No                |
| `search_files` | Searches files within the workspace | No                |
| `write_file`   | Creates or updates files            | Yes               |
| `shell`        | Executes restricted shell commands  | Yes               |
| `git_status`   | Retrieves Git working tree status   | No                |
| `git_diff`     | Retrieves Git differences           | No                |

## Agent Execution Flow

User Request
↓
Assistant API
↓
Hybrid Router
├── Direct Tool Routing
└── LLM-based Planner
↓
Tool Registry
↓
Tool Execution
↓
Tool Result / Observation
↓
Next Planning Step
↓
Final LLM Response

## Agent Capabilities

- Understands natural language requests and selects appropriate tools.
- Supports sequential execution of multiple tools.
- Uses previous tool outputs when planning subsequent actions.
- Supports up to 4 tool execution steps per request.
- Requires explicit approval for potentially destructive operations such as file creation and shell execution.
- Restricts file operations to the configured workspace.
- Applies command execution restrictions for shell operations.

## Direct Tool Execution

Tools can also be invoked directly through the API for testing and debugging.

Example:

```bash
curl -X POST http://localhost:8000/chat \
-H "Content-Type: application/json" \
-d '{"tool":{"tool":"list_files","path":"."}}'
```

## Approved File Creation

```bash
curl -X POST http://localhost:8000/chat \
-H "Content-Type: application/json" \
-d '{
  "approved": true,
  "tool": {
    "tool": "write_file",
    "path": "notes/example.txt",
    "content": "Created using the personal AI assistant"
  }
}'
```

## Approved Shell Execution

```bash
curl -X POST http://localhost:8000/chat \
-H "Content-Type: application/json" \
-d '{
  "approved": true,
  "tool": {
    "tool": "shell",
    "command": "go test ./..."
  }
}'
```

---

# Makefile Commands

| Command        | Description                             |
| -------------- | --------------------------------------- |
| `make up`      | Start services                          |
| `make down`    | Stop services                           |
| `make rebuild` | Rebuild all containers                  |
| `make logs`    | View logs                               |
| `make ps`      | View running containers                 |
| `make clean`   | Remove containers and volumes           |
| `make shell`   | Open assistant container shell          |
| `make chat`    | Test chat endpoint                      |
| `make tool`    | Test tool execution (read_file via API) |
| `make agent`   | Test the agent endpoint                 |

---
