// Minimal fixture for the internal/build Railpack integration: enough
// for Railpack's java provider to detect a Maven-built Spring Boot app
// and generate a build plan, and, once built, enough to prove the
// resulting image actually runs (see
// TestClient_BuildRailpack_Live_Java in railpack_test.go).
package com.example;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@SpringBootApplication
@RestController
public class Application {

    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }

    @GetMapping("/")
    public String index() {
        return "levelrail railpack java spring boot fixture";
    }
}
