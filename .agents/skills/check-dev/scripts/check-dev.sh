#!/bin/bash
# Netsy <https://netsy.dev>
# Copyright 2026 Nadrama Pty Ltd
# SPDX-License-Identifier: Apache-2.0

# Diagnostic script for the Netsy dev environment (make dev).
# Assumes `make dev` is running locally, which starts dev-s3 and
# netsy instance(s) via Overmind with Air hot-reload.
#
# Checks:
# 1. Overmind and dev processes are running
# 2. Health endpoints are reachable
# 3. Log files for errors/panics
# 4. Elector leadership and primary election status
# 5. Heartbeat health

ROOT_DIR="$(git rev-parse --show-toplevel)"
LOG_DIR="${ROOT_DIR}/temp/logs"

PASS_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0

pass() { PASS_COUNT=$((PASS_COUNT + 1)); echo "  ✅ $1"; }
warn() { WARN_COUNT=$((WARN_COUNT + 1)); echo "  ⚠️  $1"; }
fail() { FAIL_COUNT=$((FAIL_COUNT + 1)); echo "  ❌ $1"; }
section() { echo ""; echo "━━━ $1 ━━━"; }

# Detect instance count from OVERMIND_FORMATION or running log files
detect_instances() {
    local count=0
    for f in "${LOG_DIR}"/netsy-*.log; do
        [ -f "$f" ] && count=$((count + 1))
    done
    [ "$count" -eq 0 ] && count=1
    echo "$count"
}

INSTANCE_COUNT=$(detect_instances)

# ============================================================================
# 1. Process Checks
# ============================================================================
section "Process Checks"

PROCS_OK=true

if [ -S "${ROOT_DIR}/.overmind.sock" ]; then
    pass "Overmind socket exists"
else
    fail "Overmind socket not found — is 'make dev' running?"
    PROCS_OK=false
fi

if pgrep -f "dev-s3" > /dev/null 2>&1; then
    pass "dev-s3 process running"
else
    fail "dev-s3 process not running"
    PROCS_OK=false
fi

if pgrep -f "bin/netsy" > /dev/null 2>&1; then
    pass "netsy process running"
else
    fail "netsy process not running"
    PROCS_OK=false
fi

if [ "$PROCS_OK" = false ]; then
    echo ""
    echo "  Dev environment is not running. Start it with 'make dev'."
    echo ""
    exit 1
fi

# ============================================================================
# 2. Health Endpoint Checks
# ============================================================================
section "Health Endpoints"

check_health() {
    local name="$1"
    local url="$2"
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>/dev/null)
    if [ "$response" = "200" ]; then
        pass "$name — HTTP 200"
    else
        fail "$name — HTTP $response (expected 200)"
    fi
}

check_port() {
    local name="$1"
    local port="$2"
    if nc -z localhost "$port" 2>/dev/null; then
        pass "$name — port $port open"
    else
        fail "$name — port $port not reachable"
    fi
}

# dev-s3
check_port "dev-s3" 4566

# Per-instance checks
for i in $(seq 1 "$INSTANCE_COUNT"); do
    offset=$(( (i - 1) * 10 ))
    health_port=$(( 8080 + offset ))
    client_port=$(( 2378 + offset ))
    peer_port=$(( 2381 + offset ))

    check_health "netsy-${i} health" "http://localhost:${health_port}/health"
    check_port "netsy-${i} client gRPC" "$client_port"
    check_port "netsy-${i} peer gRPC" "$peer_port"
done

# ============================================================================
# 3. Log File Checks
# ============================================================================
section "Log Files"

check_log() {
    local name="$1"
    local logfile="$2"

    if [ ! -f "$logfile" ]; then
        fail "$name — log file not found ($logfile)"
        return
    fi

    local size
    size=$(wc -c < "$logfile" 2>/dev/null | tr -d ' ')
    if [ "$size" -eq 0 ]; then
        warn "$name — log file is empty"
        return
    fi

    local panics fatals errors
    panics=$(grep -c -i "panic" "$logfile" 2>/dev/null || true)
    fatals=$(grep -c -i "fatal" "$logfile" 2>/dev/null || true)
    errors=$(grep -c "level=ERROR" "$logfile" 2>/dev/null || true)

    if [ "$panics" -gt 0 ]; then
        fail "$name — $panics panic(s) found"
        grep -i "panic" "$logfile" | tail -3 | sed 's/^/        /'
    elif [ "$fatals" -gt 0 ]; then
        fail "$name — $fatals fatal(s) found"
        grep -i "fatal" "$logfile" | tail -3 | sed 's/^/        /'
    elif [ "$errors" -gt 0 ]; then
        warn "$name — $errors error-level log(s) found"
        grep "level=ERROR" "$logfile" | tail -3 | sed 's/^/        /'
    else
        pass "$name — no panics, fatals, or errors"
    fi
}

check_log "dev-s3" "${LOG_DIR}/dev-s3.log"
for i in $(seq 1 "$INSTANCE_COUNT"); do
    check_log "netsy-${i}" "${LOG_DIR}/netsy-${i}.log"
done

# ============================================================================
# 4. Election Status
# ============================================================================
section "Election Status"

for i in $(seq 1 "$INSTANCE_COUNT"); do
    logfile="${LOG_DIR}/netsy-${i}.log"
    [ -f "$logfile" ] || continue

    # Elector leadership
    if grep -q "acquired elector leadership" "$logfile" 2>/dev/null; then
        pass "netsy-${i} — acquired elector leadership"
    else
        # Not all nodes become elector, only check instance 1 in single-node
        if [ "$INSTANCE_COUNT" -eq 1 ]; then
            fail "netsy-${i} — did not acquire elector leadership"
        fi
    fi

    # Primary election
    if grep -q "election_completed" "$logfile" 2>/dev/null; then
        elected=$(grep "election_completed" "$logfile" | tail -1 | grep -o 'elected_node_id=[^ ]*' | cut -d= -f2)
        pass "netsy-${i} — primary elected (${elected})"
    else
        # Count recent election failures
        fail_count=$(grep -c "election_failed" "$logfile" 2>/dev/null || true)
        if [ "$fail_count" -gt 0 ]; then
            last_reason=$(grep "election_failed" "$logfile" | tail -1 | grep -o 'error="[^"]*"' | head -1)
            fail "netsy-${i} — no primary elected ($fail_count failed attempts, last: $last_reason)"
        else
            warn "netsy-${i} — no election activity found"
        fi
    fi
done

# ============================================================================
# 5. Heartbeat Health
# ============================================================================
section "Heartbeat Health"

for i in $(seq 1 "$INSTANCE_COUNT"); do
    logfile="${LOG_DIR}/netsy-${i}.log"
    [ -f "$logfile" ] || continue

    degraded_count=$(grep -c "marking node degraded" "$logfile" 2>/dev/null || true)
    hb_failures=$(grep "heartbeat" "$logfile" 2>/dev/null | grep -c -i "fail\|error" || true)
    self_degraded=$(grep -c "node self-degraded" "$logfile" 2>/dev/null || true)

    if [ "$self_degraded" -gt 0 ]; then
        # Check if it recovered (became healthy after self-degradation, e.g. after restart)
        last_health=$(grep "state_type=health" "$logfile" | tail -1 | grep -o 'new=[^ ]*' | cut -d= -f2)
        if [ "$last_health" = "healthy" ]; then
            warn "netsy-${i} — self-degraded $self_degraded time(s) but recovered to healthy"
        else
            fail "netsy-${i} — node self-degraded ($self_degraded time(s)), current state: $last_health"
        fi
    elif [ "$degraded_count" -gt 0 ]; then
        # Check if it recovered (became healthy after degradation)
        last_health=$(grep "state_type=health" "$logfile" | tail -1 | grep -o 'new=[^ ]*' | cut -d= -f2)
        if [ "$last_health" = "healthy" ]; then
            warn "netsy-${i} — was degraded $degraded_count time(s) but recovered to healthy"
        else
            fail "netsy-${i} — degraded $degraded_count time(s), current state: $last_health"
        fi
    elif [ "$hb_failures" -gt 0 ]; then
        warn "netsy-${i} — $hb_failures heartbeat failure(s) in logs"
    else
        pass "netsy-${i} — heartbeats healthy, no degradation"
    fi
done

# ============================================================================
# Summary
# ============================================================================
section "Summary"

echo ""
echo "  ✅ Passed: $PASS_COUNT"
echo "  ⚠️  Warnings: $WARN_COUNT"
echo "  ❌ Failed: $FAIL_COUNT"
echo ""

if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
else
    exit 0
fi
