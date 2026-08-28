FROM alpine:3.24 AS clickhouse_plugin

ARG CLICKHOUSE_KAFKA_CONNECT_VERSION=1.5.0
ARG CLICKHOUSE_KAFKA_CONNECT_SHA256=53e31ccd96734b8fb1f251279da17863b9b193f0764a61280461aa314050ecc6

RUN apk add --no-cache ca-certificates curl unzip
RUN set -eu; \
    plugin_archive="/tmp/clickhouse-kafka-connect.zip"; \
    curl --fail --silent --show-error --location \
      "https://github.com/ClickHouse/clickhouse-kafka-connect/releases/download/v${CLICKHOUSE_KAFKA_CONNECT_VERSION}/clickhouse-kafka-connect-v${CLICKHOUSE_KAFKA_CONNECT_VERSION}.zip" \
      --output "${plugin_archive}"; \
    echo "${CLICKHOUSE_KAFKA_CONNECT_SHA256}  ${plugin_archive}" | sha256sum -c -; \
    mkdir -p /opt/clickhouse-kafka-connect; \
    unzip -q "${plugin_archive}" -d /opt/clickhouse-kafka-connect

FROM debezium/connect:3.0.0.Final

COPY --from=clickhouse_plugin --chown=kafka:kafka \
  /opt/clickhouse-kafka-connect/ /kafka/connect/clickhouse-kafka-connect/
