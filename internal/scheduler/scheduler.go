package scheduler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/koyeb/koyeb-api-client-go/api/v1/koyeb"
	"github.com/koyeb/koyeb-github-runner-scheduler/internal/koyeb_api"
)

const (
	RunnersAppName = "github-runner"
	RunnersImage   = "koyeb/github-runner"
)

type API struct {
	koyebAPIClient koyeb_api.APIClient
	apiSecret      string
	githubToken    string
	runnersTTL     time.Duration
	cleaner        *Cleaner
}

func NewAPI(koyebClient koyeb_api.APIClient, githubToken string, apiSecret string, runnersTTL time.Duration) *API {
	return &API{
		koyebAPIClient: koyebClient,
		apiSecret:      apiSecret,
		githubToken:    githubToken,
		runnersTTL:     runnersTTL,
	}
}

// https://docs.github.com/en/webhooks/webhook-events-and-payloads
type WebHookPayload struct {
	// GitHub action ("queued", "completed", ...)
	Action     string `json:"action"`
	Repository struct {
		// Repository triggering the webhook (<owner>/<repo>)
		FullName string `json:"full_name"`
	} `json:"repository"`
	// Workflow information
	WorkflowJob struct {
		RunId        int64    `json:"run_id"`
		WorkflowName string   `json:"workflow_name"`
		Labels       []string `json:"labels"`
	} `json:"workflow_job"`
}

func (api *API) Run(port int) error {
	services, err := api.loadCurrentServices()
	if err != nil {
		return err
	}

	api.cleaner = SetupCleaner(api.koyebAPIClient, services, api.runnersTTL)

	router := http.NewServeMux()
	router.HandleFunc("/", api.scheduler)
	log.Printf("Start listening on %d\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

// loadCurrentServices lists the services in the "github-runner" Koyeb application.
func (api *API) loadCurrentServices() ([]string, error) {
	appId, err := api.koyebAPIClient.GetApp(RunnersAppName)
	if err != nil {
		return nil, err
	}
	services, err := api.koyebAPIClient.ListServices(appId)
	if err != nil {
		return nil, err
	}
	return services, err
}

// The endpoint called by GitHub webhooks. Validates the request signature, and handle the action.
func (api *API) scheduler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Allow disabling authentication for local testing
	if os.Getenv("DISABLE_AUTH") == "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if signature == "" {
			http.Error(w, "The HTTP header X-Hub-Signature-256 is missing. This API is expected to be called by GitHub webhooks.", http.StatusUnauthorized)
			return
		}

		hash := hmac.New(sha256.New, []byte(api.apiSecret))
		hash.Write(body)
		expectedSignature := "sha256=" + hex.EncodeToString(hash.Sum(nil))

		if signature != expectedSignature {
			http.Error(w, "Invalid X-Hub-Signature-256 header. Make sure the environment variable API_SECRET matches your GitHub webhook secret.", http.StatusUnauthorized)
			return
		}
	}

	var payload WebHookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Bad Request: unable to unmarshal the request body", http.StatusBadRequest)
		return
	}

	log.Printf("Received GitHub Action \"%s\" for the workflow: %s (labels: %s)\n", payload.Action, payload.WorkflowJob.WorkflowName, strings.Join(payload.WorkflowJob.Labels, ", "))
	if err := api.handleAction(&payload); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf(fmt.Sprintf("%s\n", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (api *API) handleAction(payload *WebHookPayload) error {
	var region, instanceType string

	for _, label := range payload.WorkflowJob.Labels {
		parts := strings.Split(label, "-")
		if len(parts) == 3 && parts[0] == "koyeb" {
			region = parts[1]
			instanceType = parts[2]
			break
		}
	}

	if region == "" || instanceType == "" {
		log.Printf("The action \"%s\" for the workflow %s does target this scheduler, ignoring\n", payload.Action, payload.WorkflowJob.WorkflowName)
		return nil
	}

	appId, created, err := api.koyebAPIClient.UpsertApplication(RunnersAppName)
	if err != nil {
		return err
	}
	if created {
		log.Printf("Created the Koyeb application %s (id: %s)\n", RunnersAppName, appId)
	}

	log.Printf("Checking if there is an existing %s runner on %s...\n", instanceType, region)
	serviceId, err := api.koyebAPIClient.GetService(appId, fmt.Sprintf("runner-%s-%s", region, instanceType))
	if err != nil {
		return err
	}

	if serviceId != "" {
		log.Printf("A %s runner currently exists on %s. Mark the service for removal in %s, unless a new action is received\n", instanceType, region, api.runnersTTL)
		api.cleaner.Update(serviceId)
		return nil
	}

	// If the runner does not exist but the action is not "queued", there is nothing to do
	if payload.Action != "queued" {
		log.Printf("No %s runner on %s, but the action is not \"queued\", ignoring\n", instanceType, region)
		return nil
	}

	// Queued action, start a new runner
	log.Printf("No %s runner on %s. Starting a new instance\n", instanceType, region)
	createService := koyeb.CreateService{
		AppId: koyeb.PtrString(appId),
		Definition: &koyeb.DeploymentDefinition{
			Name: koyeb.PtrString(fmt.Sprintf("runner-%s-%s", region, instanceType)),
			Type: koyeb.DEPLOYMENTDEFINITIONTYPE_WORKER.Ptr(),
			Docker: &koyeb.DockerSource{
				Image: koyeb.PtrString(RunnersImage),
			},
			Regions: []string{region},
			InstanceTypes: []koyeb.DeploymentInstanceType{
				{Type: koyeb.PtrString(instanceType), Scopes: []string{fmt.Sprintf("region:%s", region)}},
			},
			Env: []koyeb.DeploymentEnv{
				{Key: koyeb.PtrString("REPO_URL"), Value: koyeb.PtrString(fmt.Sprintf("https://github.com/%s", payload.Repository.FullName))},
				{Key: koyeb.PtrString("GITHUB_TOKEN"), Value: koyeb.PtrString(api.githubToken)},
				{Key: koyeb.PtrString("RUNNER_LABELS"), Value: koyeb.PtrString(fmt.Sprintf("koyeb-%s-%s", region, instanceType))},
			},
			Scalings: []koyeb.DeploymentScaling{
				{Min: koyeb.PtrInt64(1), Max: koyeb.PtrInt64(1), Scopes: []string{fmt.Sprintf("region:%s", region)}},
			},
		},
	}
	serviceId, err = api.koyebAPIClient.CreateService(createService)
	if err != nil {
		return err
	}
	api.cleaner.Update(serviceId)
	log.Printf("Created the service %s, marked for removal in %s\n", serviceId, api.runnersTTL)
	return nil
}
