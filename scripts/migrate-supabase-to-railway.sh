#!/usr/bin/env bash
# migrate-supabase-to-railway.sh — one-shot copy of the transcription
# database (videos, transcripts, chunks, jobs, token_usage, …) from the
# legacy Supabase pgvector instance to the new Railway pgvector instance.
#
# Why this script exists:
#   - Supabase free tier caps storage at 0.5 GB; 1M chunks of 1536-dim
#     float32 embeddings = ~6 GB, so we outgrow it fast.
#   - We already pay $20/mo on Railway and want one less vendor.
#   - pgvector data + enum types + foreign keys need to land intact.
#
# Usage:
#   export DB_SUPABASE_DSN="postgresql://postgres:...@db.<ref>.supabase.co:5432/postgres?sslmode=require"
#   export DB_RAILWAY_DSN="postgresql://postgres:...@autorack.proxy.rlwy.net:PORT/railway?sslmode=disable"
#       Railway's pgvector template does NOT terminate TLS at the Postgres
#       layer (the public proxy already encrypts the TCP tunnel), so
#       sslmode=require errors out with "SSL is not enabled on the server".
#   ./scripts/migrate-supabase-to-railway.sh                 # full migrate
#   ./scripts/migrate-supabase-to-railway.sh --schema-only   # schema first, no data
#   ./scripts/migrate-supabase-to-railway.sh --verify        # only compare row counts
#
# Safety:
#   - Bails out if pgvector extension is not available on the target.
#   - Refuses to restore over a non-empty target unless --force is set.
#   - Verifies row counts per table after restore; non-zero diff returns
#     non-zero exit status so you can wire it into CI.

set -euo pipefail

# Restrictive umask so the dump file is created mode 0600. The dump
# carries every chunk / transcript / video metadata row in the source
# database — leaving it world-readable in /tmp would expose tenant
# content to any local user on the host.
umask 077

DUMP_FILE="${DUMP_FILE:-/tmp/supabase_transcription_$(date +%Y%m%d_%H%M%S).dump}"
TABLES=(videos transcripts chunks jobs token_usage memberclass_tenant_mappings webhook_subscriptions webhook_deliveries queries analytics)
SCHEMA_ONLY=0
VERIFY_ONLY=0
FORCE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --schema-only) SCHEMA_ONLY=1; shift ;;
        --verify)      VERIFY_ONLY=1; shift ;;
        --force)       FORCE=1; shift ;;
        -h|--help)
            sed -n '2,30p' "$0"
            exit 0
            ;;
        *)
            echo "Unknown flag: $1" >&2
            exit 2
            ;;
    esac
done

require_env() {
    local var="$1"
    if [[ -z "${!var:-}" ]]; then
        echo "ERROR: \$$var must be set" >&2
        exit 2
    fi
}

require_bin() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "ERROR: '$1' not found in PATH" >&2
        exit 2
    fi
}

require_env DB_SUPABASE_DSN
require_env DB_RAILWAY_DSN
require_bin pg_dump
require_bin pg_restore
require_bin psql

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }

# ---------- 1. Preflight checks ----------

log "Pinging source..."
psql "$DB_SUPABASE_DSN" -At -c 'SELECT 1' >/dev/null
log "Pinging target..."
psql "$DB_RAILWAY_DSN"  -At -c 'SELECT 1' >/dev/null

log "Checking pgvector availability on target..."
PGVECTOR_AVAILABLE=$(psql "$DB_RAILWAY_DSN" -At -c \
    "SELECT count(*) FROM pg_available_extensions WHERE name = 'vector';")
if [[ "$PGVECTOR_AVAILABLE" != "1" ]]; then
    echo "ERROR: pgvector extension is NOT available on the Railway target." >&2
    echo "       Recreate the service using the 'PostgreSQL pgvector' template." >&2
    exit 1
fi

log "Ensuring vector extension exists on target..."
psql "$DB_RAILWAY_DSN" -c 'CREATE EXTENSION IF NOT EXISTS vector;' >/dev/null

# ---------- 2. Source row counts ----------

declare -A SRC_COUNTS
log "Counting source rows..."
for t in "${TABLES[@]}"; do
    if psql "$DB_SUPABASE_DSN" -At -c "SELECT to_regclass('public.$t') IS NOT NULL;" | grep -q t; then
        n=$(psql "$DB_SUPABASE_DSN" -At -c "SELECT count(*) FROM public.$t;")
        SRC_COUNTS[$t]=$n
        printf '    %-40s %s\n' "$t" "$n"
    else
        SRC_COUNTS[$t]="MISSING"
        printf '    %-40s (table not present in source — skipping)\n' "$t"
    fi
done

if [[ "$VERIFY_ONLY" == "1" ]]; then
    log "Verify-only mode — diffing against target."
    rc=0
    for t in "${TABLES[@]}"; do
        [[ "${SRC_COUNTS[$t]}" == "MISSING" ]] && continue
        tgt=$(psql "$DB_RAILWAY_DSN" -At -c "SELECT count(*) FROM public.$t;" 2>/dev/null || echo "MISSING")
        if [[ "$tgt" != "${SRC_COUNTS[$t]}" ]]; then
            printf 'DIFF %-40s src=%s tgt=%s\n' "$t" "${SRC_COUNTS[$t]}" "$tgt"
            rc=1
        fi
    done
    exit $rc
fi

# ---------- 3. Refuse to overwrite a populated target ----------

if [[ "$FORCE" != "1" ]]; then
    TGT_CHUNKS=$(psql "$DB_RAILWAY_DSN" -At -c \
        "SELECT to_regclass('public.chunks') IS NULL OR (SELECT count(*) FROM public.chunks) = 0;" \
        2>/dev/null || echo t)
    if [[ "$TGT_CHUNKS" != "t" ]]; then
        echo "ERROR: target already has rows in 'chunks'. Re-run with --force to clobber." >&2
        exit 1
    fi
fi

# ---------- 4. Dump source ----------

DUMP_ARGS=(--no-owner --no-acl --format=custom --file="$DUMP_FILE")
if [[ "$SCHEMA_ONLY" == "1" ]]; then
    DUMP_ARGS+=(--schema-only)
fi
# Restrict to the application schema. Supabase clusters carry vault.* +
# graphql.* + extensions.* objects we cannot recreate on a vanilla pgvector
# image (pg_net, pg_graphql, supabase_vault are Supabase-only extensions).
DUMP_ARGS+=(--schema=public)
# Avoid dumping _prisma_migrations (Prisma bookkeeping; not used on Railway).
DUMP_ARGS+=(--exclude-table=public._prisma_migrations)

log "Dumping source -> $DUMP_FILE ..."
pg_dump "$DB_SUPABASE_DSN" "${DUMP_ARGS[@]}"
ls -lh "$DUMP_FILE"

# ---------- 5. Restore into target ----------

RESTORE_ARGS=(--no-owner --no-acl --dbname="$DB_RAILWAY_DSN")
if [[ "$FORCE" == "1" ]]; then
    RESTORE_ARGS+=(--clean --if-exists)
fi

log "Restoring into Railway target ..."
# Don't pass -1 (single transaction) because pg_restore needs to keep going
# past benign "extension already exists" notices that abort a single tx.
# pg_restore exits non-zero whenever ANY statement errors, even when the
# remainder succeeded (e.g. Supabase-only extensions in a legacy dump).
# Capture status, log it, then defer the go/no-go to the row-count diff
# below so the verification path is reachable.
set +e
pg_restore "${RESTORE_ARGS[@]}" "$DUMP_FILE"
RESTORE_RC=$?
set -e
if [[ "$RESTORE_RC" != "0" ]]; then
    log "pg_restore exited $RESTORE_RC (likely Supabase-only extensions/objects); continuing to row-count verify."
fi

# ---------- 6. Post-restore verification ----------

log "Verifying row counts ..."
rc=0
for t in "${TABLES[@]}"; do
    [[ "${SRC_COUNTS[$t]}" == "MISSING" ]] && continue
    tgt=$(psql "$DB_RAILWAY_DSN" -At -c "SELECT count(*) FROM public.$t;" 2>/dev/null || echo "MISSING")
    if [[ "$tgt" == "${SRC_COUNTS[$t]}" ]]; then
        printf '  OK   %-40s %s\n' "$t" "$tgt"
    else
        printf '  DIFF %-40s src=%s tgt=%s\n' "$t" "${SRC_COUNTS[$t]}" "$tgt"
        rc=1
    fi
done

log "Sampling chunks.embedding vector dimension on target..."
DIM=$(psql "$DB_RAILWAY_DSN" -At -c \
    "SELECT vector_dims(embedding) FROM chunks WHERE embedding IS NOT NULL LIMIT 1;" 2>/dev/null || echo "")
if [[ -n "$DIM" ]]; then
    printf '  embedding dimension: %s (expected 1536 for text-embedding-3-small)\n' "$DIM"
    if [[ "$DIM" != "1536" ]]; then
        echo "  WARN: dimension drift — verify the legacy data was produced with text-embedding-3-small." >&2
    fi
else
    log "  no embedding rows to sample (empty chunks or all NULL)."
fi

if [[ "$rc" != "0" ]]; then
    echo "FAILED: row-count diff detected; investigate before pointing the app at Railway." >&2
    exit "$rc"
fi

log "Migration complete. Set DB_TRANSCRIPTION_DSN to the Railway internal DSN and redeploy."
