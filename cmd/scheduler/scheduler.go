package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/koyeb/koyeb-github-runner-scheduler/internal/koyeb_api"
	"github.com/koyeb/koyeb-github-runner-scheduler/internal/scheduler"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	rootCmd := &cobra.Command{
		// Validate flags
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if viper.GetInt("port") == 0 {
				return fmt.Errorf("PORT or --port must be omitted or valid")
			}
			if viper.GetString("koyeb-token") == "" {
				return fmt.Errorf("KOYEB_TOKEN or --koyeb-token must be set to a valid Koyeb API token used to create runners")
			}
			if viper.GetString("api-secret") == "" {
				return fmt.Errorf("API_SECRET or --api-secret must be set to a valid secret used to authenticate webhook requests")
			}
			if viper.GetInt("runners-ttl") == 0 {
				return fmt.Errorf("RUNNERS_TTL or --runners-ttl must be omitted or a valid integer representing a duration in minutes")
			}
			if viper.GetString("mode") != "repository" && viper.GetString("mode") != "organization" {
				return fmt.Errorf("MODE or --mode must be omitted or set to repository (default) or organization")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			koyebClient := koyeb_api.NewAPIClient(viper.GetString("koyeb-token"))

			apiMode := scheduler.RepositoryMode
			if viper.GetString("mode") == "organization" {
				apiMode = scheduler.OrganizationMode
			}

			scheduler := scheduler.NewAPI(
				scheduler.APIParams{
					KoyebAPIClient:      koyebClient,
					GithubToken:         viper.GetString("github-token"),
					ApiSecret:           viper.GetString("api-secret"),
					RunnersTTL:          time.Duration(viper.GetInt("runners-ttl")) * time.Minute,
					DisableDockerDaemon: viper.GetBool("disable-docker-daemon"),
					Mode:                apiMode,
				},
			)
			return scheduler.Run(viper.GetInt("port"))
		},
	}

	rootCmd.Flags().IntP("port", "p", 8000, "Port to listen on")
	viper.BindPFlag("port", rootCmd.Flags().Lookup("port"))
	viper.BindEnv("port", "PORT")

	rootCmd.Flags().String("koyeb-token", "", "Koyeb API token")
	viper.BindPFlag("koyeb-token", rootCmd.Flags().Lookup("koyeb-token"))
	viper.BindEnv("koyeb-token", "KOYEB_TOKEN")

	rootCmd.Flags().String("github-token", "", "GitHub API token")
	viper.BindPFlag("github-token", rootCmd.Flags().Lookup("github-token"))
	viper.BindEnv("github-token", "GITHUB_TOKEN")

	rootCmd.Flags().String("api-secret", "", "API secret")
	viper.BindPFlag("api-secret", rootCmd.Flags().Lookup("api-secret"))
	viper.BindEnv("api-secret", "API_SECRET")

	rootCmd.Flags().Int("runners-ttl", 120, "Runners TTL in minutes")
	viper.BindPFlag("runners-ttl", rootCmd.Flags().Lookup("runners-ttl"))
	viper.BindEnv("runners-ttl", "RUNNERS_TTL")

	rootCmd.Flags().Bool("disable-docker-daemon", false, "Disable Docker daemon")
	viper.BindPFlag("disable-docker-daemon", rootCmd.Flags().Lookup("disable-docker-daemon"))
	viper.BindEnv("disable-docker-daemon", "DISABLE_DOCKER_DAEMON")

	rootCmd.Flags().StringP("mode", "m", "repository", "Scheduler mode (repository or organization)")
	viper.BindPFlag("mode", rootCmd.Flags().Lookup("mode"))
	viper.BindEnv("mode", "MODE")

	if err := rootCmd.Execute(); err != nil {
		log.Printf("%s\n", err)
		os.Exit(1)
	}
}
