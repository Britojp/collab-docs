FROM maven:3.9-eclipse-temurin-21 AS builder
WORKDIR /app
COPY pom.xml .
COPY spell-worker/pom.xml spell-worker/pom.xml
COPY user-service/pom.xml user-service/pom.xml
COPY audit-worker/pom.xml audit-worker/pom.xml
COPY notif-worker/pom.xml notif-worker/pom.xml
RUN --mount=type=cache,target=/root/.m2 mvn -pl audit-worker dependency:go-offline -B
COPY audit-worker/src audit-worker/src
RUN --mount=type=cache,target=/root/.m2 mvn -pl audit-worker package -DskipTests -B

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/audit-worker/target/*.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]
