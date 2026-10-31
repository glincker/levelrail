package catalog

import "embed"

// gamesFS holds game manifests with an EULA prompt. They sit apart from
// manifests/ because a game server has no HTTP domain or health path.
//
//go:embed games
var gamesFS embed.FS

// catalogGamesTemplates are dedicated game servers. Each image was checked
// to publish linux/amd64 and linux/arm64 before it was added.
var catalogGamesTemplates = append(mustLoadGameManifests(), catalogGamesPlain...)

func mustLoadGameManifests() []Template {
	ms, err := LoadManifests(gamesFS, "games")
	if err != nil {
		panic(err)
	}
	out := make([]Template, 0, len(ms))
	for _, m := range ms {
		t, err := m.Template()
		if err != nil {
			panic(err)
		}
		out = append(out, t)
	}
	return out
}

var catalogGamesPlain = []Template{
	{
		ID:     "factorio",
		Name:   "Factorio Server",
		Slogan: "A dedicated Factorio multiplayer server with saves kept in a volume.",
		Description: "Players connect on UDP 34197: the port binds privately by default, so set a public bind in the app's Network settings and open UDP 34197 in the host and cloud firewall.\n" +
			"A new map is generated on first start. Edit server-settings.json under the data volume to set the name, password and visibility.\n" +
			"RCON is not published. Image is multi-arch (amd64 and arm64).",
		Category:               "Games",
		DocumentationURL:       "https://github.com/factoriotools/factorio-docker",
		RecommendedMemoryBytes: 2147483648, // 2Gi
		Compose: `services:
  factorio:
    image: factoriotools/factorio:2.1.24
    ports:
      - "34197:34197/udp"
    volumes:
      - factorio_data:/factorio
    healthcheck:
      test: ["CMD-SHELL", "grep -qi ':8595 ' /proc/net/udp /proc/net/udp6"]
      interval: 30s
      timeout: 5s
      retries: 5
      start_period: 60s
`,
	},
	{
		ID:     "terraria",
		Name:   "Terraria Server",
		Slogan: "A vanilla Terraria dedicated server that creates a world on first start.",
		Description: "Players connect on TCP 7777: the port binds privately by default, so set a public bind in the app's Network settings and open TCP 7777 in the host and cloud firewall.\n" +
			"The world is created on first start and kept in the data volume. The image has no password by default: add one by overriding the start command with -password.\n" +
			"Image is multi-arch (amd64 and arm64).",
		Category:               "Games",
		DocumentationURL:       "https://github.com/Ryshe/terraria-docker",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  terraria:
    image: ryshe/terraria:vanilla-1.4.5.8
    ports:
      - "7777:7777"
    command: ["-world", "/root/.local/share/Terraria/Worlds/world.wld", "-autocreate", "2", "-worldname", "world", "-maxplayers", "8"]
    stdin_open: true
    tty: true
    volumes:
      - terraria_data:/root/.local/share/Terraria/Worlds
    healthcheck:
      test: ["CMD-SHELL", "grep -qi ':1E61 ' /proc/net/tcp /proc/net/tcp6"]
      interval: 30s
      timeout: 5s
      retries: 5
      start_period: 60s
`,
	},
	{
		ID:     "palworld",
		Name:   "Palworld Server",
		Slogan: "A dedicated Palworld server with a generated admin password and RCON kept internal.",
		Description: "Players connect on UDP 8211: the port binds privately by default, so set a public bind in the app's Network settings and open UDP 8211 in the host and cloud firewall.\n" +
			"The admin password is generated; read it in the service environment settings. Set a join password in the data volume's PalWorldSettings.ini.\n" +
			"Needs about 16 GB of RAM for a few players, so it does not fit a 12 GB host that also runs other apps. RCON is not published.\n" +
			"Image is multi-arch (amd64 and arm64); the game itself is x86 software, so on arm64 it runs under emulation and is slow.",
		Category:               "Games",
		DocumentationURL:       "https://github.com/thijsvl/palworld-server-docker",
		RecommendedMemoryBytes: 17179869184, // 16Gi
		Compose: `services:
  palworld:
    image: thijsvanloef/palworld-server-docker:v2.8.1
    ports:
      - "8211:8211/udp"
    environment:
      PLAYERS: "8"
      PORT: "8211"
      MULTITHREADING: "true"
      RCON_ENABLED: "true"
      ADMIN_PASSWORD: $SERVICE_PASSWORD_PALADMIN
    volumes:
      - palworld_data:/palworld
    healthcheck:
      test: ["CMD-SHELL", "grep -qi ':2013 ' /proc/net/udp /proc/net/udp6"]
      interval: 30s
      timeout: 5s
      retries: 5
      start_period: 300s
`,
	},
}
