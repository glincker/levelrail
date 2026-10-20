package catalog

var analyticsTemplates = []Template{
	{
		ID:                     "metabase",
		Name:                   "Metabase",
		Slogan:                 "Ask questions of your data and share dashboards, no SQL required.",
		Category:               "Analytics",
		DocumentationURL:       "https://www.metabase.com/docs/latest/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  metabase:
    image: metabase/metabase:v0.50.8
    ports: ["3000:3000"]
    environment:
      MB_DB_FILE: /metabase-data/metabase.db
    volumes:
      - metabase_data:/metabase-data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
}
