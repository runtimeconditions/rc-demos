FROM maven:3.9-eclipse-temurin-17 AS build

WORKDIR /src
COPY pom.xml ./pom.xml
RUN mvn -q -B dependency:go-offline
COPY src ./src
RUN mvn -q -B package dependency:copy-dependencies -DoutputDirectory=target/lib

FROM eclipse-temurin:17-jre

COPY --from=build /src/target/classes /app/classes
COPY --from=build /src/target/lib /app/lib

ENTRYPOINT ["java", "-cp", "/app/classes:/app/lib/*", "io.runtimeconditions.demos.requestlogger.RequestLoggerHttp"]
