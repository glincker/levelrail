FROM eclipse-temurin:21-jdk AS build
WORKDIR /src
COPY . .
RUN if [ -f mvnw ]; then ./mvnw -q -DskipTests package; else ./gradlew -q bootJar -x test; fi
RUN mkdir /out && cp $(ls target/*.jar build/libs/*.jar 2>/dev/null | grep -v plain | head -n 1) /out/app.jar \
    && cd /out && java -Djarmode=tools -jar app.jar extract --layers --destination layers

FROM eclipse-temurin:21-jre
WORKDIR /app
RUN useradd --system --uid 10001 app
COPY --from=build /out/layers/dependencies/ ./
COPY --from=build /out/layers/spring-boot-loader/ ./
COPY --from=build /out/layers/snapshot-dependencies/ ./
COPY --from=build /out/layers/application/ ./
ENV JAVA_TOOL_OPTIONS="-XX:MaxRAMPercentage=75" SERVER_PORT=8080
USER app
EXPOSE 8080
ENTRYPOINT ["java", "-jar", "app.jar"]
