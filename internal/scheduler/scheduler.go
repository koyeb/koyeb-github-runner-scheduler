package scheduler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
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

type APIMode int

const (
	RepositoryMode APIMode = iota
	OrganizationMode
)

type APIParams struct {
	KoyebAPIClient      koyeb_api.APIClient
	ApiSecret           string
	GithubToken         string
	RunnersTTL          time.Duration
	DisableDockerDaemon bool
	Mode                APIMode
	// To start a runner, the label must be composed as "<prefix>-<region>-<instanceType>". By default, the prefix is "koyeb".
	Prefix string
}

type API struct {
	params  APIParams
	cleaner *Cleaner
}

func NewAPI(params APIParams) *API {
	return &API{params: params}
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

	api.cleaner = SetupCleaner(api.params.KoyebAPIClient, services, api.params.RunnersTTL)

	router := http.NewServeMux()
	router.HandleFunc("/", api.scheduler)
	log.Printf("Start listening on %d\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

// loadCurrentServices lists the services in the "github-runner" Koyeb application.
func (api *API) loadCurrentServices() ([]string, error) {
	appId, err := api.params.KoyebAPIClient.GetApp(RunnersAppName)
	if err != nil {
		return nil, err
	}
	services, err := api.params.KoyebAPIClient.ListServices(appId)
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

		hash := hmac.New(sha256.New, []byte(api.params.ApiSecret))
		hash.Write(body)
		expectedSignature := "sha256=" + hex.EncodeToString(hash.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSignature)) != 1 {
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
		if len(parts) == 3 && parts[0] == api.params.Prefix {
			region = parts[1]
			instanceType = parts[2]
			break
		}
	}

	if region == "" || instanceType == "" {
		log.Printf("The action \"%s\" for the workflow %s does target this scheduler, ignoring\n", payload.Action, payload.WorkflowJob.WorkflowName)
		return nil
	}

	appId, created, err := api.params.KoyebAPIClient.UpsertApplication(RunnersAppName)
	if err != nil {
		return err
	}
	if created {
		log.Printf("Created the Koyeb application %s (id: %s)\n", RunnersAppName, appId)
	}

	log.Printf("Checking if there is an existing %s runner on %s...\n", instanceType, region)
	serviceId, err := api.params.KoyebAPIClient.GetService(appId, fmt.Sprintf("runner-%s-%s", region, instanceType))
	if err != nil {
		return err
	}

	if serviceId != "" {
		log.Printf("A %s runner currently exists on %s. Mark the service for removal in %s, unless a new action is received\n", instanceType, region, api.params.RunnersTTL)
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

	var repoUrl string
	switch api.params.Mode {
	case RepositoryMode:
		repoUrl = fmt.Sprintf("https://github.com/%s", payload.Repository.FullName)
	case OrganizationMode:
		repoUrl = fmt.Sprintf("https://github.com/%s", strings.Split(payload.Repository.FullName, "/")[0])
	}

	createService := koyeb.CreateService{
		AppId: koyeb.PtrString(appId),
		Definition: &koyeb.DeploymentDefinition{
			Name: koyeb.PtrString(fmt.Sprintf("runner-%s-%s", region, instanceType)),
			Type: koyeb.DEPLOYMENTDEFINITIONTYPE_WORKER.Ptr(),
			Docker: &koyeb.DockerSource{
				Image:      koyeb.PtrString(RunnersImage),
				Privileged: koyeb.PtrBool(true),
			},
			Regions: []string{region},
			InstanceTypes: []koyeb.DeploymentInstanceType{
				{Type: koyeb.PtrString(instanceType), Scopes: []string{fmt.Sprintf("region:%s", region)}},
			},
			Env: []koyeb.DeploymentEnv{
				{Key: koyeb.PtrString("REPO_URL"), Value: koyeb.PtrString(repoUrl)},
				{Key: koyeb.PtrString("GITHUB_TOKEN"), Value: koyeb.PtrString(api.params.GithubToken)},
				{Key: koyeb.PtrString("RUNNER_LABELS"), Value: koyeb.PtrString(fmt.Sprintf("%s-%s-%s", api.params.Prefix, region, instanceType))},
			},
			Scalings: []koyeb.DeploymentScaling{
				{Min: koyeb.PtrInt64(1), Max: koyeb.PtrInt64(1), Scopes: []string{fmt.Sprintf("region:%s", region)}},
			},
		},
	}
	if api.params.DisableDockerDaemon {
		// The container doesn't have to be privileged if the Docker daemon is disabled
		createService.Definition.Docker.Privileged = koyeb.PtrBool(false)
		createService.Definition.Env = append(
			createService.Definition.Env,
			koyeb.DeploymentEnv{Key: koyeb.PtrString("DISABLE_DOCKER_DAEMON"), Value: koyeb.PtrString("true")},
		)
	}

	serviceId, err = api.params.KoyebAPIClient.CreateService(createService)
	if err != nil {
		return err
	}
	api.cleaner.Update(serviceId)
	log.Printf("Created the service %s, marked for removal in %s\n", serviceId, api.params.RunnersTTL)
	return nil
}
