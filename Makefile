.PHONY: up down rebuild logs ps clean chat build test dev
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
test:
	cd assistant-api && go test ./...

.PHONY: frontend-dev frontend-build frontend-test
frontend-dev:
	cd frontend && npm run dev
frontend-build:
	cd frontend && npm run build
frontend-test:
	cd frontend && npm test
