.DEFAULT_GOAL := help

.PHONY: help install dev dev-backend dev-frontend build build-frontend build-backend test test-backend lint clean docker-build docker-up docker-down docker-logs docker-compose docker-compose-down docker-compose-logs

# Colors for help output
CYAN := \033[36m
RESET := \033[0m

help: ## Показать список доступных команд
	@echo "Доступные команды:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-18s$(RESET) %s\n", $$1, $$2}'

install: ## Установить все зависимости (backend и frontend)
	@echo "Установка зависимостей бэкенда..."
	cd backend && go mod download
	@echo "Установка зависимостей фронтенда..."
	cd frontend && npm install

dev: ## Запустить одновременно Go backend (:8080) и Vite dev-сервер (:5173)
	@echo "Запуск backend (порт 8080) и frontend (порт 5173)..."
	@trap 'kill 0' EXIT INT TERM; \
	(cd backend && go run .) & \
	(cd frontend && npm run dev) & \
	wait

dev-backend: ## Запустить Go backend локально (порт 8080)
	cd backend && go run .

dev-frontend: ## Запустить Vite dev-сервер фронтенда (порт 5173)
	cd frontend && npm run dev

build-frontend: ## Собрать статический production бандл фронтенда (frontend/dist)
	cd frontend && npm run build

build-backend: ## Скомпилировать бинарник бэкенда (bin/avari)
	@mkdir -p bin
	cd backend && CGO_ENABLED=0 go build -o ../bin/avari .

build: build-frontend build-backend ## Собрать фронтенд и бэкенд

test-backend: ## Запустить тесты бэкенда
	cd backend && go test -v -race ./...

test: test-backend ## Запустить все тесты

lint: ## Проверить типы TypeScript и форматирование Go
	cd frontend && npx tsc --noEmit
	cd backend && go vet ./...

clean: ## Очистить скомпилированные файлы и кэш
	rm -rf bin frontend/dist
	cd backend && go clean

DOCKER_COMPOSE ?= docker compose

docker-build: ## Собрать Docker-образ приложения
	docker build -t avari-domains .

docker-up: ## Запустить приложение через Docker Compose в фоне
	$(DOCKER_COMPOSE) up -d --build

docker-down: ## Остановить контейнеры Docker Compose
	$(DOCKER_COMPOSE) down

docker-logs: ## Просмотреть логи запущенных Docker-контейнеров
	$(DOCKER_COMPOSE) logs -f

docker-compose: docker-up ## Запустить контейнеры через Docker Compose (синоним docker-up)

docker-compose-down: docker-down ## Остановить контейнеры Docker Compose (синоним docker-down)

docker-compose-logs: docker-logs ## Просмотреть логи Docker Compose (синоним docker-logs)
