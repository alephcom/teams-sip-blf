#!/usr/bin/env bash
# Build and run the CUCM JTAPI sidecar.
# Requires: JDK 17+, lib/jtapi.jar from CiscoJTAPILinux.zip, Maven 3.8+
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

if [[ ! -f lib/jtapi.jar ]]; then
  echo "Missing lib/jtapi.jar — see lib/README.md" >&2
  exit 1
fi

mvn -q -DskipTests package

# Include all jars under lib/ (Cisco zip may ship companion jars)
CP="target/cucm-jtapi-0.1.0.jar"
for j in lib/*.jar; do
  CP="$CP:$j"
done

exec java -cp "$CP" com.alephcom.teams.cucmjtapi.Main "$@"
