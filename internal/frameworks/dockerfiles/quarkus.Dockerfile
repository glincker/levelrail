FROM eclipse-temurin:21-jdk AS build
WORKDIR /src
COPY . .
RUN if [ -f mvnw ]; then ./mvnw -q -DskipTests package; else ./gradlew -q build -x test; fi
RUN mkdir /out && cp -r $(ls -d target/quarkus-app build/quarkus-app 2>/dev/null | head -n 1)/. /out/

FROM eclipse-temurin:21-jre
WORKDIR /app
RUN useradd --system --uid 10001 app
COPY --from=build /out/ ./
ENV JAVA_TOOL_OPTIONS="-XX:MaxRAMPercentage=75" QUARKUS_HTTP_HOST=0.0.0.0 QUARKUS_HTTP_PORT=8080
USER app
EXPOSE 8080
ENTRYPOINT ["java", "-jar", "quarkus-run.jar"]
