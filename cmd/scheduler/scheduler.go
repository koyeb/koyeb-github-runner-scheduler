package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/koyeb/koyeb-github-runner-scheduler/internal/koyeb_api"
	"github.com/koyeb/koyeb-github-runner-scheduler/internal/scheduler"
)

func main() {
	port := 8000
	if portStr := os.Getenv("PORT"); portStr != "" {
		var err error

		port, err = strconv.Atoi(portStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid PORT value: %s\n", portStr)
		}

	}

	koyebToken := os.Getenv("KOYEB_TOKEN")
	if koyebToken == "" {
		fmt.Fprintf(os.Stderr, "Missing environment variable KOYEB_TOKEN\n")
		os.Exit(1)
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		fmt.Fprintf(os.Stderr, "Missing environment variable GITHUB_TOKEN\n")
		os.Exit(1)
	}

	apiSecret := os.Getenv("API_SECRET")
	if apiSecret == "" {
		fmt.Fprintf(os.Stderr, "Missing environment variable API_SECRET\n")
		os.Exit(1)
	}

	koyebClient := koyeb_api.NewAPIClient(koyebToken)
	scheduler := scheduler.NewAPI(koyebClient, githubToken, apiSecret)
	if err := scheduler.Run(port); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
