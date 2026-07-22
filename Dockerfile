# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/hoyo-codes .

FROM alpine:3.20
# ca-certificates: HTTPS to the upstream APIs and the Discord webhook.
# tzdata: so the schedule's timezone (TZ env or config `timezone`) resolves.
RUN apk add --no-cache ca-certificates tzdata \
 && mkdir -p /app/data && chown 1000:1000 /app/data
WORKDIR /app
COPY --from=build /out/hoyo-codes /usr/local/bin/hoyo-codes
# Config is mounted at /app/config.yaml (see docker-compose.yaml).
USER 1000:1000
ENTRYPOINT ["hoyo-codes"]
