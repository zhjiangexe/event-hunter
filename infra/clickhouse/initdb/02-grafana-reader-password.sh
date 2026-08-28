#!/bin/sh
set -eu

# The SQL bootstrap keeps a deterministic local-development password. A fresh
# hardening install rotates it before Grafana starts, without writing the
# deployment secret into a committed SQL file.
if [ "${GRAFANA_CLICKHOUSE_PASSWORD:-grafana_reader_local_only}" = "grafana_reader_local_only" ]; then
  exit 0
fi

clickhouse-client \
  --user "${CLICKHOUSE_USER}" \
  --password "${CLICKHOUSE_PASSWORD}" \
  --param_reader_password "${GRAFANA_CLICKHOUSE_PASSWORD}" \
  --query "ALTER USER grafana_reader IDENTIFIED WITH sha256_password BY {reader_password:String}"
