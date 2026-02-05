# Setup & Installation Guide

ACORDE is designed to run everywhere. The easiest way to run it on "any other system" (Windows, Mac, Servers) is via Docker.

## 1. 🐳 Run via Docker (Recommended)

Requires [Docker Desktop](https://www.docker.com/products/docker-desktop/) or Docker Engine.

### Option A: Docker Compose (Easiest)
This will start a node with data persisting to `./data`.

```bash
docker-compose up -d
```

- **P2P Port**: `4001` (Exposed to host)
- **API Port**: `7331` (Exposed to host)
- **Logs**: `docker-compose logs -f`

### Option B: Manual Docker Run
If you don't have Compose, you can build and run directly:

```bash
# Build image
docker build -t acorde .

# Run container
docker run -d \
  --name acorde-node \
  -p 4001:4001 \
  -p 7331:7331 \
  -v $(pwd)/data:/home/acorde/data \
  acorde:latest
```

### ✨ Features of this Docker Setup
- **Secure**: Runs as non-root user `acorde`.
- **Tiny**: Based on Alpine Linux (~20MB).
- **Production Ready**: Includes Healthchecks and Timezone data.
- **Persistent**: Data stored in `./data`.

---

## 2. 🛠️ Build from Source

If you prefer to run the binary directly, you must have:
- **Go** 1.25+
- **GCC** (C Compiler) - *Required for SQLite*

### Linux / macOS
```bash
# Clone
git clone https://github.com/amaydixit11/acorde.git
cd acorde

# Build
make release

# Run
./build/acorde daemon
```

### Windows (WSL2 Recommended)
We recommend using **WSL2** (Ubuntu) on Windows to avoid C compiler issues.
1. Install WSL2 (`wsl --install`)
2. Follow Linux instructions above.

If you must use native Windows, install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) first, then run:

```powershell
go build -o acorde.exe ./cmd/acorde
.\acorde.exe daemon
```
