set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Cores para diferenciar saída de cada serviço
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; MAGENTA='\033[0;35m'; NC='\033[0m'

log() { echo -e "${1}[${2}]${NC} ${3}"; }

# Mata todos os processos filhos ao sair (Ctrl+C ou erro)
cleanup() {
  echo ""
  log "$RED" "dev" "encerrando todos os serviços..."
  kill 0
}
trap cleanup EXIT INT TERM

# Rodando infra
log "$YELLOW" "infra" "subindo postgres, redis, kafka..."
docker compose -f "$ROOT/infra/docker-compose.yml" \
  up -d postgres redis zookeeper kafka kafka-init

log "$YELLOW" "infra" "aguardando postgres ficar saudável..."
until docker compose -f "$ROOT/infra/docker-compose.yml" \
  exec -T postgres pg_isready -U collabdocs &>/dev/null; do
  sleep 1
done

log "$YELLOW" "infra" "aguardando kafka ficar saudável..."
until docker compose -f "$ROOT/infra/docker-compose.yml" \
  exec -T kafka kafka-topics --bootstrap-server localhost:9092 --list &>/dev/null; do
  sleep 2
done

log "$GREEN" "infra" "pronta"

# Compilando Java
log "$CYAN" "java" "compilando módulos (mvn install)..."
mvn -f "$ROOT/java/pom.xml" install -DskipTests -q
log "$GREEN" "java" "build ok"

# Rodando serviços

prefix() {
  local color="$1" name="$2"
  while IFS= read -r line; do
    echo -e "${color}[${name}]${NC} ${line}"
  done
}

# Go
(cd "$ROOT" && go run ./go/gateway/cmd/...     2>&1 | prefix "$BLUE"    "gateway      ") &
(cd "$ROOT" && go run ./go/doc-service/cmd/... 2>&1 | prefix "$MAGENTA" "doc-service  ") &
(cd "$ROOT" && go run ./go/collab-service/cmd/... 2>&1 | prefix "$CYAN" "collab-svc   ") &

# Java
(mvn -f "$ROOT/java/pom.xml" -pl user-service  spring-boot:run -q 2>&1 | prefix "$GREEN"  "user-service ") &
(mvn -f "$ROOT/java/pom.xml" -pl spell-worker  spring-boot:run -q 2>&1 | prefix "$YELLOW" "spell-worker ") &
(mvn -f "$ROOT/java/pom.xml" -pl notif-worker  spring-boot:run -q 2>&1 | prefix "$RED"    "notif-worker ") &
(mvn -f "$ROOT/java/pom.xml" -pl audit-worker  spring-boot:run -q 2>&1 | prefix "$CYAN"   "audit-worker ") &

log "$GREEN" "dev" "todos os serviços iniciados — Ctrl+C para encerrar"

wait
