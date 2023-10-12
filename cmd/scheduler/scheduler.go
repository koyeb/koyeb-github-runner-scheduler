package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/koyeb/koyeb-github-runner-scheduler/internal/koyeb_api"
	"github.com/koyeb/koyeb-github-runner-scheduler/internal/scheduler"
)

func main() {
	port := 8000
	if portStr := os.Getenv("PORT"); portStr != "" {
		var err error

		port, err = strconv.Atoi(portStr)
		if err != nil {
			log.Printf("Invalid PORT value: %s\n", portStr)
		}
	}

	koyebToken := os.Getenv("KOYEB_TOKEN")
	if koyebToken == "" {
		log.Printf("Missing environment variable KOYEB_TOKEN\n")
		os.Exit(1)
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Printf("Missing environment variable GITHUB_TOKEN\n")
		os.Exit(1)
	}

	apiSecret := os.Getenv("API_SECRET")
	if apiSecret == "" {
		log.Printf("Missing environment variable API_SECRET\n")
		os.Exit(1)
	}

	runnersTTL := 120 * time.Minute
	if envTTL := os.Getenv("RUNNERS_TTL"); envTTL != "" {
		var err error

		intTTL, err := strconv.Atoi(envTTL)
		if err != nil {
			log.Printf("Invalid RUNNERS_TTL value: %s\n", envTTL)
			os.Exit(1)
		}
		runnersTTL = time.Duration(intTTL) * time.Minute
	}

	disableDockerDaemon := false
	if envDisableDockerDaemon := os.Getenv("DISABLE_DOCKER_DAEMON"); envDisableDockerDaemon != "" {
		disableDockerDaemon = true
	}

	koyebClient := koyeb_api.NewAPIClient(koyebToken)
	scheduler := scheduler.NewAPI(koyebClient, githubToken, apiSecret, runnersTTL, disableDockerDaemon)
	if err := scheduler.Run(port); err != nil {
		log.Printf("%s\n", err)
		os.Exit(1)
	}
}
