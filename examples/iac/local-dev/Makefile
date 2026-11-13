# Local control plane: export APP_EXPERIMENTAL=iac APP_API_URL=http://localhost:8080 and a token first.
VARS = --var-file .env.local

.PHONY: plan apply
plan:
	levelrail-cli diff -f app.yaml $(VARS)

apply:
	levelrail-cli apply -f app.yaml $(VARS) --yes
