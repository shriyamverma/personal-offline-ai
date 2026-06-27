.PHONY: up down rebuild logs ps clean chat shell

build:
	docker compose build assistant-api

up:
	docker compose up

dev:
	docker compose up -d --build assistant-api

down:
	docker compose down

rebuild:
	docker compose down
	docker compose up --build -d

logs:
	docker compose logs -f

ps:
	docker compose ps

clean:
	docker compose down -v

chat:
	curl -X POST http://localhost:8000/chat \
	-H "Content-Type: application/json" \
	-d '{"prompt":"What is docker"}'

tool:
	curl -X POST http://localhost:8000/chat \
	-H "Content-Type: application/json" \
	-d '{"prompt":"read file","tool":{"tool":"read_file","path":"assistant-api/go.mod"}}'

agent:
	curl -X POST http://localhost:8000/chat \
    -H "Content-Type: application/json" \
    -d '{"prompt":"Summarize assistant-api/go.mod"}'

# it is NOT real shell execution tool, just container access
shell:
	docker exec -it assistant-api sh