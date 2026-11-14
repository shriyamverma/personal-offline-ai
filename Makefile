.PHONY: up down rebuild logs ps clean chat tool agent shell build test dev
build:
	docker compose build assistant-api frontend
up:
	docker compose up

dev:
	docker compose up -d --build assistant-api
down:
	docker compose down
rebuild:
	docker compose down
	docker compose up -d --build
logs:
	docker compose logs -f
ps:
	docker compose ps
clean:
	docker compose down -v
chat:
	curl -sS -X POST http://localhost:8000/chat -H "Content-Type: application/json" -d '{"prompt":"What is docker"}'
tool:
	curl -sS -X POST http://localhost:8000/chat -H "Content-Type: application/json" -d '{"prompt":"ignore","tool":{"tool":"read_file","path":"assistant-api/main.go"}}'
agent:
	curl -sS -X POST http://localhost:8000/chat -H "Content-Type: application/json" -d '{"prompt":"Inspect assistant-api directory and summarize the main components"}'
test:
	cd assistant-api && go test ./...
shell:
	docker exec -it assistant-api sh

.PHONY: frontend-dev frontend-build frontend-test
frontend-dev:
	cd frontend && npm run dev
frontend-build:
	cd frontend && npm run build
frontend-test:
	cd frontend && npm test
