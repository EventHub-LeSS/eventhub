# Build stage
FROM golang:1.26.5-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -o seed ./cmd/seed
RUN CGO_ENABLED=0 GOOS=linux go build -o seed-live ./cmd/seed-live

FROM alpine:latest
WORKDIR /root/
# Which deployment this image serves: "ref" (demo/mock data) or "live". CI bakes the
# flavor per pipeline (dev-cicd: ref, live-cicd: live), so ./seed seeds the right
# content without any configuration in the deployment stacks — the same pattern as
# the per-deployment realm file. A DEPLOYMENT environment variable (local dev
# compose) overrides the baked value.
ARG DEPLOYMENT=ref
ENV DEPLOYMENT=${DEPLOYMENT}
COPY --from=builder /app/api .
COPY --from=builder /app/migrate .
COPY --from=builder /app/seed .
COPY --from=builder /app/seed-live .
COPY --from=builder /app/migrations ./migrations
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD test -f /root/api || exit 1
# Run the application
CMD ["./api"]
