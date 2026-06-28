#!/bin/bash
# Chaos: Disconnect PostgreSQL from the network and reconnect
echo "=== CHAOS: Network Partition (PostgreSQL) ==="
NETWORK=${1:-deploy_internal}
echo "Network: $NETWORK"
echo "Disconnecting..."
docker network disconnect $NETWORK deploy-postgres-1 2>&1
sleep 2
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'DB: {d[\"database\"]}, Proxy: {d[\"proxy\"]}')"
echo "Waiting 30s..."
sleep 30
echo "Reconnecting..."
docker network connect $NETWORK deploy-postgres-1 2>&1
sleep 3
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'DB: {d[\"database\"]}, Proxy: {d[\"proxy\"]}')"
echo "Network partition chaos complete."
