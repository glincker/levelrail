FROM alpine:3.20

# Deliberately long-running, not a one-shot echo: this fixture stands in
# for a real service in deploy_live_test.go, which needs the container
# to still be up when it checks Running, not exited from finishing an
# instant command before the assertion runs.
CMD ["sleep", "infinity"]
