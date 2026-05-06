# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go binary (frontend dist must exist for //go:embed all:dist)
FROM golang:1.26-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN go build -o .build/auto-prd ./cmd/auto-prd

# Stage 3: E2E test runner
FROM mcr.microsoft.com/playwright:v1.50.0-noble
WORKDIR /app/e2e

RUN apt-get update && apt-get install -y git && rm -rf /var/lib/apt/lists/*

RUN git config --global user.email "e2e@test.local" \
 && git config --global user.name "E2E Test" \
 && git config --global init.defaultBranch main

COPY --from=backend /app/.build/auto-prd /usr/local/bin/auto-prd

COPY e2e/package.json e2e/package-lock.json ./
RUN npm ci
RUN npx playwright install chromium --with-deps

COPY e2e/ .

CMD ["npx", "playwright", "test"]
