#!/usr/bin/env bash
# Uso:
#   ./scripts/new-java-module.sh <nome-do-modulo> [porta]
#
# Exemplos:
#   ./scripts/new-java-module.sh spell-worker          # worker sem porta HTTP
#   ./scripts/new-java-module.sh notification-service 8082  # serviço com porta HTTP
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Argumentos:
MODULE="${1:-}"
PORT="${2:-}"

if [[ -z "$MODULE" ]]; then
  echo "Uso: $0 <nome-do-modulo> [porta]"
  exit 1
fi

MODULE_DIR="$ROOT/java/$MODULE"

if [[ -d "$MODULE_DIR" ]]; then
  echo "Módulo '$MODULE' já existe em java/$MODULE"
  exit 1
fi

# Package Java: primeira palavra antes do hífen (ex: spell-worker → spell)
PKG="${MODULE%%-*}"
PKG_PATH="br/ufg/collabdocs/$PKG"
PKG_NAME="br.ufg.collabdocs.$PKG"
CLASS="$(python3 -c "s='$PKG'; print(s[0].upper()+s[1:])""")Application"

IS_SERVICE=false
[[ -n "$PORT" ]] && IS_SERVICE=true

echo "→ criando java/$MODULE (pacote: $PKG_NAME, porta: ${PORT:-nenhuma})"

# 1. Estrutura de pastas 
mkdir -p "$MODULE_DIR/src/main/java/$PKG_PATH"
mkdir -p "$MODULE_DIR/src/main/resources"
mkdir -p "$MODULE_DIR/src/test/java/$PKG_PATH"

# 2. pom.xml do módulo 
cat > "$MODULE_DIR/pom.xml" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <parent>
        <groupId>br.ufg.collabdocs</groupId>
        <artifactId>collabdocs-java</artifactId>
        <version>0.0.1-SNAPSHOT</version>
        <relativePath>../pom.xml</relativePath>
    </parent>

    <artifactId>$MODULE</artifactId>
    <name>$MODULE</name>

    <dependencies>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter</artifactId>
        </dependency>
        <dependency>
            <groupId>org.springframework.kafka</groupId>
            <artifactId>spring-kafka</artifactId>
        </dependency>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-test</artifactId>
            <scope>test</scope>
        </dependency>
    </dependencies>

    <build>
        <plugins>
            <plugin>
                <groupId>org.springframework.boot</groupId>
                <artifactId>spring-boot-maven-plugin</artifactId>
            </plugin>
        </plugins>
    </build>
</project>
EOF

# 3. Application.java 
cat > "$MODULE_DIR/src/main/java/$PKG_PATH/$CLASS.java" <<EOF
package $PKG_NAME;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class $CLASS {
    public static void main(String[] args) {
        SpringApplication.run($CLASS.class, args);
    }
}
EOF

# 4. application.yml 
{
  if $IS_SERVICE; then
    printf "server:\n  port: \${SERVER_PORT:%s}\n\n" "$PORT"
  fi
  cat <<EOF
spring:
  application:
    name: $MODULE
  kafka:
    bootstrap-servers: \${SPRING_KAFKA_BOOTSTRAP_SERVERS:localhost:9092}
    consumer:
      group-id: $MODULE
      auto-offset-reset: earliest
      key-deserializer: org.apache.kafka.common.serialization.StringDeserializer
      value-deserializer: org.springframework.kafka.support.serializer.JsonDeserializer
      properties:
        spring.json.trusted.packages: "*"
EOF
} > "$MODULE_DIR/src/main/resources/application.yml"

# 5. Dockerfile 
EXISTING_MODULES=$(ls "$ROOT/java/" | grep -v "^pom.xml$" | grep -v "^$MODULE$" || true)

{
  echo "FROM maven:3.9-eclipse-temurin-21 AS builder"
  echo "WORKDIR /app"
  echo "COPY pom.xml ."
  for mod in $EXISTING_MODULES $MODULE; do
    echo "COPY $mod/pom.xml $mod/pom.xml"
  done
  echo "RUN --mount=type=cache,target=/root/.m2 mvn -pl $MODULE dependency:go-offline -B"
  echo "COPY $MODULE/src $MODULE/src"
  echo "RUN --mount=type=cache,target=/root/.m2 mvn -pl $MODULE package -DskipTests -B"
  echo ""
  echo "FROM eclipse-temurin:21-jre-alpine"
  echo "WORKDIR /app"
  echo "COPY --from=builder /app/$MODULE/target/*.jar app.jar"
  $IS_SERVICE && echo "EXPOSE $PORT"
  echo "ENTRYPOINT [\"java\", \"-jar\", \"app.jar\"]"
} > "$MODULE_DIR/Dockerfile"

# 6. Atualiza java/pom.xml (Python — sem problemas de sed multiline) 
python3 - "$ROOT/java/pom.xml" "$MODULE" <<'PYEOF'
import sys, re
path, module = sys.argv[1], sys.argv[2]
content = open(path).read()
content = content.replace('    </modules>', f'        <module>{module}</module>\n    </modules>')
open(path, 'w').write(content)
PYEOF

# 7. Atualiza Dockerfiles existentes 
for mod in $EXISTING_MODULES; do
  DOCKERFILE="$ROOT/java/$mod/Dockerfile"
  [[ -f "$DOCKERFILE" ]] || continue
  python3 - "$DOCKERFILE" "$MODULE" <<'PYEOF'
import sys
path, module = sys.argv[1], sys.argv[2]
lines = open(path).readlines()
out = []
for line in lines:
    if ('RUN mvn' in line or 'RUN --mount' in line) and 'dependency:go-offline' in line:
        out.append(f'COPY {module}/pom.xml {module}/pom.xml\n')
    out.append(line)
open(path, 'w').writelines(out)
PYEOF
done

# 8. Atualiza docker-compose.yml (Python) 
python3 - "$ROOT/infra/docker-compose.yml" "$MODULE" "$PORT" "$IS_SERVICE" <<'PYEOF'
import sys
path, module, port, is_service_str = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
is_service = is_service_str == 'true'

lines = [
    f'\n  {module}:\n',
    f'    build:\n',
    f'      context: ../java\n',
    f'      dockerfile: {module}/Dockerfile\n',
    f'    restart: unless-stopped\n',
    f'    depends_on:\n',
]
if is_service:
    lines.append('      postgres:\n')
    lines.append('        condition: service_healthy\n')
lines += [
    '      kafka:\n',
    '        condition: service_healthy\n',
]
if is_service:
    lines += [
        f'    ports:\n',
        f'      - "{port}:{port}"\n',
    ]
lines.append('    environment:\n')
lines.append('      SPRING_KAFKA_BOOTSTRAP_SERVERS: kafka:29092\n')
if is_service:
    lines += [
        '      SPRING_DATASOURCE_URL: jdbc:postgresql://postgres:5432/collabdocs\n',
        '      SPRING_DATASOURCE_USERNAME: collabdocs\n',
        '      SPRING_DATASOURCE_PASSWORD: collabdocs\n',
    ]

content = open(path).read()
content = content.replace('\nvolumes:', ''.join(lines) + '\nvolumes:')
open(path, 'w').write(content)
PYEOF

echo ""
echo "✓ java/$MODULE criado"
echo "✓ java/pom.xml atualizado"
echo "✓ Dockerfiles existentes atualizados"
echo "✓ infra/docker-compose.yml atualizado"
echo ""
echo "Próximos passos:"
echo "  1. Adicione dependências em java/$MODULE/pom.xml"
echo "  2. Implemente a lógica em java/$MODULE/src/main/java/$PKG_PATH/"
echo "  3. make up  (ou  docker compose build $MODULE && docker compose up -d $MODULE)"
