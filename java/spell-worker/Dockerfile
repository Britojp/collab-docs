FROM maven:3.9-eclipse-temurin-21 AS builder
WORKDIR /app
COPY pom.xml .
COPY user-service/pom.xml user-service/pom.xml
COPY spell-worker/pom.xml spell-worker/pom.xml
COPY audit-worker/pom.xml audit-worker/pom.xml
COPY notif-worker/pom.xml notif-worker/pom.xml
RUN --mount=type=cache,target=/root/.m2 mvn -pl spell-worker dependency:go-offline -B
COPY spell-worker/src spell-worker/src
RUN --mount=type=cache,target=/root/.m2 mvn -pl spell-worker package -DskipTests -B

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/spell-worker/target/*.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]
