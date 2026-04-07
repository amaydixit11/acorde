#!/bin/bash

# Multi-node Sync Test Script for ACORDE
# This script automates the process of starting two nodes,
# pairing them, and verifying data synchronization.

set -e

# Configuration
NODE1_DIR="/tmp/acorde-test1"
NODE2_DIR="/tmp/acorde-test2"
NODE1_P2P_PORT=4001
NODE2_P2P_PORT=4002
NODE1_API_PORT=7331
NODE2_API_PORT=7332

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Starting ACORDE Multi-node Sync Test...${NC}"

# Cleanup previous tests
rm -rf "$NODE1_DIR" "$NODE2_DIR"
mkdir -p "$NODE1_DIR" "$NODE2_DIR"

# Build acorde
echo "🏗️ Building acorde..."
go build -o acorde ./cmd/acorde

# Start Node 1 in background
echo "🟢 Starting Node 1 (API: $NODE1_API_PORT)..."
./acorde daemon --name node1 --data "$NODE1_DIR" --port $NODE1_P2P_PORT --api-port $NODE1_API_PORT --verbose > node1.log 2>&1 &
NODE1_PID=$!

# Start Node 2 in background
echo "🟢 Starting Node 2 (API: $NODE2_API_PORT)..."
./acorde daemon --name node2 --data "$NODE2_DIR" --port $NODE2_P2P_PORT --api-port $NODE2_API_PORT --verbose > node2.log 2>&1 &
NODE2_PID=$!

# Function to cleanup on exit
cleanup() {
    echo -e "\n🛑 Cleaning up..."
    kill $NODE1_PID $NODE2_PID 2>/dev/null || true
    # rm acorde
    echo "👋 Done."
}
trap cleanup EXIT

# Wait for nodes to start
echo "⏳ Waiting for nodes to initialize..."
sleep 3

# Pair nodes
echo "🔗 Pairing nodes..."
# Node 1 generates invite
# Capture the full line and then extract the part starting with acorde://
INVITE_CODE=$(./acorde invite --data "$NODE1_DIR" --port 4003 | grep "Full code (for CLI):" | sed 's/.*Full code (for CLI): //')

if [ -z "$INVITE_CODE" ]; then
    echo "❌ Failed to generate invite code"
    exit 1
fi

echo "🤝 Node 2 joining with code: ${INVITE_CODE:0:20}..."
./acorde pair --data "$NODE2_DIR" "$INVITE_CODE"

# Wait for P2P connection
echo "⏳ Waiting for P2P connection (mDNS)..."
sleep 5

# Step 1: Add entry to Node 1 via API (to ensure daemon sees it)
echo -e "${BLUE}📝 Adding entry to Node 1...${NC}"
ENTRY_CONTENT="Distruibuted Sync Test $(date)"
curl -s -X POST "http://localhost:$NODE1_API_PORT/entries" \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"note\",\"content\":\"$ENTRY_CONTENT\",\"tags\":[\"test\",\"sync\"],\"public\":true}" > /dev/null

echo "✅ Entry added to Node 1 via API."

# Step 2: Poll Node 2 for the entry
echo -e "${BLUE}🔍 Polling Node 2 for data...${NC}"
MAX_RETRIES=15
RETRY_COUNT=0
FOUND=false

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    # Using CLI list on Node 2 is fine as it reads the DB updated by Node 2 daemon
    RESPONSE=$(./acorde list --data "$NODE2_DIR")
    if echo "$RESPONSE" | grep -q "Distruibuted Sync Test"; then
        FOUND=true
        break
    fi
    echo "   ...not found yet, retrying ($((RETRY_COUNT+1))/$MAX_RETRIES)..."
    sleep 2
    RETRY_COUNT=$((RETRY_COUNT+1))
done

if [ "$FOUND" = true ]; then
    echo -e "${GREEN}⭐ SUCCESS: Data synced from Node 1 to Node 2!${NC}"
else
    echo "❌ FAILURE: Data did not sync within timeout."
    ./acorde list --data "$NODE2_DIR"
    exit 1
fi

# Step 3: Conflict Test (Concurrent update)
echo -e "${BLUE}🧨 Testing Conflict Resolution (LWW)...${NC}"

# Get Node 2's PeerID for authorization
NODE2_PEERID=$(./acorde status --data "$NODE2_DIR" | grep "Local Peer ID:" | awk '{print $4}')
echo "👤 Node 2 PeerID: $NODE2_PEERID"

# Add a shared entry via Node 1 API
curl -s -X POST "http://localhost:$NODE1_API_PORT/entries" \
  -H "Content-Type: application/json" \
  -d '{"type":"note","content":"Initial Content","tags":["conflict"],"public":true}' > entry.json

ENTRY_ID=$(grep -o '"id":"[^"]*' entry.json | cut -d'"' -f4)
if [ -z "$ENTRY_ID" ]; then
    ENTRY_ID=$(grep -o '"id": "[^"]*' entry.json | cut -d'"' -f4)
fi

# Authorize Node 2 to WRITE to this entry
echo "🔑 Authorizing Node 2 on Node 1..."
curl -s -X POST "http://localhost:$NODE1_API_PORT/entries/$ENTRY_ID/authorize" \
  -H "Content-Type: application/json" \
  -d "{\"peer_id\":\"$NODE2_PEERID\"}"

echo "⏳ Waiting for ACL sync (10s)..."
sleep 10

echo "⏳ Waiting for initial sync of entry $ENTRY_ID on Node 2..."
RETRY_COUNT=0
while [ $RETRY_COUNT -lt 15 ]; do
    if ./acorde get --data "$NODE2_DIR" "$ENTRY_ID" >/dev/null 2>&1; then
        break
    fi
    sleep 2
    RETRY_COUNT=$((RETRY_COUNT+1))
done

# Node 1 updates
echo "Node 1 updating..."
curl -s -X PUT "http://localhost:$NODE1_API_PORT/entries/$ENTRY_ID" \
  -H "Content-Type: application/json" \
  -d '{"content":"Update from A"}'

# Node 2 updates (nearly concurrently)
echo "Node 2 updating..."
curl -s -X PUT "http://localhost:$NODE2_API_PORT/entries/$ENTRY_ID" \
  -H "Content-Type: application/json" \
  -d '{"content":"Update from B"}'

echo "⏳ Waiting for convergence..."
sleep 8

CONTENT1=$(./acorde get --data "$NODE1_DIR" "$ENTRY_ID" | grep '"content":' | cut -d'"' -f4)
CONTENT2=$(./acorde get --data "$NODE2_DIR" "$ENTRY_ID" | grep '"content":' | cut -d'"' -f4)

if [ "$CONTENT1" = "$CONTENT2" ]; then
    echo -e "${GREEN}✅ CONVERGED: Both nodes reached state: $CONTENT1${NC}"
else
    echo "❌ DIVERGED: Node 1 has '$CONTENT1', Node 2 has '$CONTENT2'"
    exit 1
fi

echo -e "${GREEN}🎊 All distributed property tests passed!${NC}"
