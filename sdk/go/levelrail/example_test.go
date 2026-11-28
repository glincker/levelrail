package levelrail_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/glincker/levelrail/sdk/go/levelrail"
)

// exampleURL is a placeholder. Point LEVELRAIL_API_URL at a real control plane.
const exampleURL = "https://levelrail.example.com"

func newExampleClient() *levelrail.Client {
	return levelrail.New(exampleURL, os.Getenv("LEVELRAIL_TOKEN"))
}

func ExampleNew() {
	client := levelrail.New(exampleURL, os.Getenv("LEVELRAIL_TOKEN"),
		levelrail.WithTimeout(10*time.Second))
	health, err := client.Health(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(health)
}

func ExampleClient_ListApps() {
	client := newExampleClient()
	apps, next, err := client.ListApps(context.Background(), levelrail.ListOptions{Limit: 50})
	if err != nil {
		log.Fatal(err)
	}
	for _, app := range apps {
		fmt.Println(app["name"])
	}
	if next != "" {
		fmt.Println("next cursor:", next)
	}
}

func ExampleClient_AllApps() {
	client := newExampleClient()
	for app, err := range client.AllApps(context.Background(), levelrail.ListOptions{Limit: 100}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(app["name"])
	}
}

func ExamplePaginate() {
	client := newExampleClient()
	fetch := func(ctx context.Context, cursor string) ([]levelrail.Object, string, error) {
		return client.ListApps(ctx, levelrail.ListOptions{Limit: 50, Cursor: cursor})
	}
	for app, err := range levelrail.Paginate(context.Background(), fetch) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(app["name"])
	}
}

func ExampleClient_Changes() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newExampleClient()
	for ev, err := range client.Changes(ctx) {
		if err != nil {
			// Reconnect and refetch state on error or end of stream.
			log.Print(err)
			return
		}
		if ev.Heartbeat {
			continue
		}
		fmt.Println("changed:", ev.Topics)
	}
}

func ExampleWithIdempotencyKey() {
	ctx := levelrail.WithIdempotencyKey(context.Background(), "deploy-web-2026-11-24")
	client := newExampleClient()
	deploy, err := client.DeployApp(ctx, "web", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(deploy)
}
