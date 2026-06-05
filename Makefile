.PHONY: up down rebuild logs ps clean chat shell

up:
	docker compose up -d

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
	-d '{"prompt":"Hello"}'

shell:
	docker exec -it assistant-api sh