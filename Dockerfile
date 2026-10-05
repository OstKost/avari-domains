FROM alpine:3.21
RUN addgroup -S avari && adduser -S -G avari avari && mkdir /data /app && chown -R avari:avari /data /app
COPY bin/avari /app/avari
COPY frontend/dist /app/dist
RUN chmod 755 /app/avari && chown -R avari:avari /app
USER avari
ENV SQLITE_PATH=/data/avari.sqlite STATIC_DIR=/app/dist LISTEN_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1
CMD ["/app/avari"]
