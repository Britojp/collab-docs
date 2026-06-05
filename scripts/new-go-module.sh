#!/usr/bin/env bash
# Uso:
#   ./scripts/new-go-module.sh <nome-do-modulo> [porta]
#
# Exemplos:
#   ./scripts/new-go-module.sh metrics-worker          # worker sem porta HTTP
#   ./scripts/new-go-module.sh analytics-service 8082  # serviço HTTP
#   ./scripts/new-go-module.sh rpc-service 50053       # serviço gRPC
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

MODULE="${1:-}"
PORT="${2:-}"

if [[ -z "$MODULE" ]]; then
  echo "Uso: $0 <nome-do-modulo> [porta]"
  exit 1
fi

MODULE_DIR="$ROOT/go/$MODULE"

if [[ -d "$MODULE_DIR" ]]; then
  echo "Módulo '$MODULE' já existe em go/$MODULE"
  exit 1
fi

BINARY="$MODULE"
HAS_PORT=false
[[ -n "$PORT" ]] && HAS_PORT=true

echo "→ criando go/$MODULE (porta: ${PORT:-nenhuma})"

# 1. Estrutura de pastas
mkdir -p "$MODULE_DIR/cmd"
mkdir -p "$MODULE_DIR/internal"
mkdir -p "$MODULE_DIR/tests"

# 2. go.mod
cat > "$MODULE_DIR/go.mod" <<EOF
module github.com/britojp/collabdocs/go/$MODULE

go 1.25.0
EOF

# 3. cmd/main.go
if $HAS_PORT; then
  cat > "$MODULE_DIR/cmd/main.go" <<EOF
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "$PORT"
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	log.Printf("$MODULE listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
EOF
else
  cat > "$MODULE_DIR/cmd/main.go" <<EOF
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("$MODULE started")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("$MODULE shutting down")
}
EOF
fi

# 4. tests/module_test.go
cat > "$MODULE_DIR/tests/module_test.go" <<EOF
package tests

import "testing"

func TestPlaceholder(t *testing.T) {
	// TODO: adicionar testes
}
EOF

# 5. Dockerfile
{
  echo "FROM golang:1.25-alpine AS builder"
  echo "WORKDIR /app"
  echo "COPY go.mod go.sum* ./"
  echo "RUN go mod download"
  echo "COPY . ."
  echo "RUN CGO_ENABLED=0 go build -o /$BINARY ./cmd/main.go"
  echo ""
  echo "FROM alpine:3.19"
  echo "RUN apk --no-cache add ca-certificates tzdata"
  echo "WORKDIR /app"
  echo "COPY --from=builder /$BINARY ./$BINARY"
  $HAS_PORT && echo "EXPOSE $PORT"
  echo "CMD [\"./$BINARY\"]"
} > "$MODULE_DIR/Dockerfile"

# 6. Atualiza go.work (Python — insere antes do ')' final do bloco use)
python3 - "$ROOT/go.work" "$MODULE" <<'PYEOF'
import sys
path, module = sys.argv[1], sys.argv[2]
content = open(path).read().rstrip()
idx = content.rfind('\n)')
content = content[:idx] + f'\n\t./go/{module}' + content[idx:]
open(path, 'w').write(content + '\n')
PYEOF

# 7. Atualiza docker-compose.yml (Python)
python3 - "$ROOT/infra/docker-compose.yml" "$MODULE" "$PORT" "$HAS_PORT" <<'PYEOF'
import sys
path, module, port, has_port_str = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
has_port = has_port_str == 'true'

lines = [
    f'\n  {module}:\n',
    f'    build:\n',
    f'      context: ../go/{module}\n',
    f'    restart: unless-stopped\n',
    f'    depends_on:\n',
    f'      redis:\n',
    f'        condition: service_healthy\n',
]
if has_port:
    lines += [
        f'    ports:\n',
        f'      - "{port}:{port}"\n',
    ]
lines.append('    environment:\n')
lines.append(f'      PORT: {port}\n' if has_port else f'      # adicione variáveis de ambiente aqui\n')

content = open(path).read()
content = content.replace('\nvolumes:', ''.join(lines) + '\nvolumes:')
open(path, 'w').write(content)
PYEOF

echo ""
echo "✓ go/$MODULE criado"
echo "✓ go.work atualizado"
echo "✓ infra/docker-compose.yml atualizado"
echo ""
echo "Próximos passos:"
echo "  1. Implemente a lógica em go/$MODULE/cmd/main.go"
echo "  2. Adicione pacotes internos em go/$MODULE/internal/"
echo "  3. Adicione dependências: cd go/$MODULE && go get <pacote>"
echo "  4. make up  (ou  docker compose build $MODULE && docker compose up -d $MODULE)"
