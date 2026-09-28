FROM node:24-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-alpine AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /avari .

FROM alpine:3.22
RUN addgroup -S avari && adduser -S -G avari avari && mkdir /data /app && chown avari:avari /data
COPY --from=backend /avari /app/avari
COPY --from=frontend /app/frontend/dist /app/dist
USER avari
ENV SQLITE_PATH=/data/avari.sqlite STATIC_DIR=/app/dist LISTEN_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1
CMD ["/app/avari"]
