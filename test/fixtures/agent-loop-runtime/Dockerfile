# Runtime-error fixture for test/e2e/agent_loop_test.go: builds fine, but the
# container exits at start until GREETING is set.
FROM busybox:1.36
COPY start.sh /start.sh
EXPOSE 8080
CMD ["sh", "/start.sh"]
