FROM eclipse-temurin:21-jdk AS build
WORKDIR /src
COPY . .
RUN if [ -f mvnw ]; then ./mvnw -q -DskipTests package; else ./gradlew -q shadowJar -x test; fi
RUN mkdir /out && cp $(ls target/*.jar build/libs/*-all.jar 2>/dev/null | grep -v original | head -n 1) /out/app.jar

FROM eclipse-temurin:21-jre
WORKDIR /app
RUN useradd --system --uid 10001 app
COPY --from=build /out/app.jar ./app.jar
ENV JAVA_TOOL_OPTIONS="-XX:MaxRAMPercentage=75" MICRONAUT_SERVER_PORT=8080
USER app
EXPOSE 8080
ENTRYPOINT ["java", "-jar", "app.jar"]
