package main

import (
	"context"
	"flag"
	"log"

	"github.com/GLINCKER/levelrail/terraform-provider/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "serve with a debugger attached")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: provider.RegistryAddress,
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
