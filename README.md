# AIDevOps Plug-and-Play Monitoring System

> M.Tech (Software Engineering) dissertation — BITS Pilani WILP, SEZG628T.

A self-hosted monitoring platform. Any service sends logs and metrics using standard OpenTelemetry; a Go ingestion server authenticates, queues and stores them in MongoDB; an AI layer explains serious errors.

## Status
Mid-semester phase: ingestion pipeline (custom JSON + OTLP, logs + metrics).

## Repository layout
| Path | Contents |
|---|---|
| `server/` | Go ingestion server |
| `dashboard/` | Web dashboard (later phase) |
| `tests/` | Ingestion test scripts and load tests |

## Run locally
```bash
cp .env.example .env
make up      # MongoDB
make run     # Go server on :8080
```

## Author
Aditya Kasralkar — 2024TM93619
