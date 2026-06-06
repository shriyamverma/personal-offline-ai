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

---

# Architecture

```text
User
  ↓
Open WebUI / API Client
  ↓
Assistant API (Go)
  ↓
Memory Layer (SQLite + Embeddings)
  ↓
Ollama
  ↓
Qwen2.5:3B
````

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
│   │   └── memory/
│   ├── Dockerfile
│   ├── go.mod
│   └── main.go
│
├── ollama/
│   └── Dockerfile
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

* Docker Desktop
* Go (optional for local development)

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

```text
assistant-api-running
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

---

# Memory System

The assistant stores conversation history using SQLite.

Current capabilities:
- Stores user messages
- Stores assistant responses
- Builds short-term conversational context from last N messages

---

# Tool System

This project includes a pluggable tool execution system.

## Current tools:
- read_file: Reads files from workspace directory
- list_files: Lists files and directories inside workspace
- (future) shell execution (planned but disabled for safety)

## Architecture:
User → API → Tool Router → Tool Registry → Tool Execution → Response

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

---
