FROM maven:3.9-eclipse-temurin-21 AS builder
WORKDIR /app
COPY pom.xml .
COPY user-service/pom.xml user-service/pom.xml
COPY spell-worker/pom.xml spell-worker/pom.xml
COPY audit-worker/pom.xml audit-worker/pom.xml
COPY notif-worker/pom.xml notif-worker/pom.xml
RUN --mount=type=cache,target=/root/.m2 mvn -pl user-service dependency:go-offline -B
COPY user-service/src user-service/src
RUN --mount=type=cache,target=/root/.m2 mvn -pl user-service package -DskipTests -B

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/user-service/target/*.jar app.jar
EXPOSE 8081
ENTRYPOINT ["java", "-jar", "app.jar"]
