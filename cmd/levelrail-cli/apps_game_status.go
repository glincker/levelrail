package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runAppsGameStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps game-status"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n\nWhether a game server answers on its published TCP port, with version, players, latency and the address players join. Minecraft Java is read with the server list ping, other games with a TCP connect.\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(*flag.FlagSet) {})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	res, err := c.client.GetAppGameStatus(context.Background(), app)
	if err != nil {
		return c.fail(fmt.Errorf("get game status for %q: %w", app, err))
	}
	return c.render(res, func() { printGameStatus(stdout, res) })
}

func printGameStatus(w io.Writer, res apiclient.AppGameStatus) {
	switch res.Exposure {
	case "none":
		_, _ = fmt.Fprintf(w, "%s: no TCP game port is published\n", res.App)
		return
	case "remote_node":
		_, _ = fmt.Fprintf(w, "%s: runs on a remote node, status ping is not available\n", res.App)
		return
	}
	state := "offline"
	if res.Online {
		state = "online"
	}
	_, _ = fmt.Fprintf(w, "%s: %s", res.App, state)
	if res.Kind == "minecraft" {
		_, _ = fmt.Fprintf(w, ", %s, %d/%d players", res.Version, res.Players, res.MaxPlayers)
	}
	if res.Online {
		_, _ = fmt.Fprintf(w, ", %d ms", res.LatencyMS)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, joinLine(res))
}

// joinLine tells players where to connect, or why there is no address yet.
func joinLine(res apiclient.AppGameStatus) string {
	switch {
	case res.JoinAddress != "":
		return "Join: " + res.JoinAddress
	case res.JoinPort > 0:
		return fmt.Sprintf("Join: <this server's address>:%d (set APP_PUBLIC_HOST to show it here)", res.JoinPort)
	default:
		return "Join: not exposed (publish the port with apps streams create or apps exposure)"
	}
}
