package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/koyeb/koyeb-github-runner-executor/internal/executor"
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

	api := executor.API{}

	if koyebToken := os.Getenv("KOYEB_TOKEN"); koyebToken == "" {
		fmt.Fprintf(os.Stderr, "Missing environment variable KOYEB_TOKEN\n")
		os.Exit(1)
	} else {
		api.KoyebToken = koyebToken
	}

	if githubToken := os.Getenv("GITHUB_TOKEN"); githubToken == "" {
		fmt.Fprintf(os.Stderr, "Missing environment variable GITHUB_TOKEN\n")
		os.Exit(1)
	} else {
		api.GithubToken = githubToken
	}

	if apiSecret := os.Getenv("API_SECRET"); apiSecret == "" {
		fmt.Fprintf(os.Stderr, "Missing environment variable API_SECRET\n")
		os.Exit(1)
	} else {
		api.APISecret = apiSecret
	}

	if err := api.Run(port); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
