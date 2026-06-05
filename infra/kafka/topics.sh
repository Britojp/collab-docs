#!/usr/bin/env bash
set -euo pipefail

BOOTSTRAP="${KAFKA_BOOTSTRAP:-localhost:9092}"
RF="${REPLICATION_FACTOR:-3}"

create() {
    local topic=$1 partitions=$2 retention_ms=$3
    kafka-topics --bootstrap-server "$BOOTSTRAP" \
        --create --if-not-exists \
        --topic "$topic" \
        --partitions "$partitions" \
        --replication-factor "$RF" \
        --config "retention.ms=${retention_ms}"
    echo "  created: $topic"
}

echo "Creating Kafka topics..."
create "doc.ops"     8 604800000
create "doc.events"  8 86400000
create "notif.users" 4 259200000
create "audit.log"   4 2592000000
echo "Done."
