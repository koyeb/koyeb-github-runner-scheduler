package scheduler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/koyeb/koyeb-api-client-go/api/v1/koyeb"
	"github.com/koyeb/koyeb-github-runner-scheduler/internal/koyeb_api"
)

const (
	AppName = "github-runner"
)

type API struct {
	KoyebAPIClient koyeb_api.APIClient
	APISecret      string
	GithubToken    string
}

func NewAPI(koyebClient koyeb_api.APIClient, githubToken string, apiSecret string) API {
	return API{koyebClient, apiSecret, githubToken}
}

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

func (api API) Run(port int) error {
	router := http.NewServeMux()
	router.HandleFunc("/", api.scheduler)
	fmt.Printf("Start listening on %d\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

func (api API) scheduler(w http.ResponseWriter, r *http.Request) {
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

		hash := hmac.New(sha256.New, []byte(api.APISecret))
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

	switch payload.Action {
	case "queued":
		fmt.Printf(">> Received GitHub Action: %s/%s\n", payload.WorkflowJob.WorkflowName, payload.Action)
		if err := api.startRunner(&payload); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			fmt.Fprint(os.Stderr, fmt.Sprintf("%s\n", err))
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		fmt.Printf(">> Ignoring GitHub action: %s/%s\n", payload.WorkflowJob.WorkflowName, payload.Action)
		w.WriteHeader(http.StatusOK)
	}
}

func (api API) startRunner(payload *WebHookPayload) error {
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
		fmt.Printf("%s/%s does not have a label targeting this scheduler, ignoring\n", payload.WorkflowJob.WorkflowName, payload.Action)
		return nil
	}

	appId, created, err := api.KoyebAPIClient.UpsertApplication(AppName)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("Application %s (%s) created\n", AppName, appId)
	}

	fmt.Printf("Check if there is an existing %s runner on %s\n", instanceType, region)
	serviceId, err := api.KoyebAPIClient.GetService(appId, fmt.Sprintf("runner-%s-%s", region, instanceType))
	if err != nil {
		return err
	}

	if serviceId != "" {
		fmt.Printf("A %s runner is already running on %s, ignoring\n", instanceType, region)
		return nil
	}

	fmt.Printf("Starting %s runner on %s\n", instanceType, region)
	createService := koyeb.CreateService{
		AppId: koyeb.PtrString(appId),
		Definition: &koyeb.DeploymentDefinition{
			Name: koyeb.PtrString(fmt.Sprintf("runner-%s-%s", region, instanceType)),
			Type: koyeb.DEPLOYMENTDEFINITIONTYPE_WORKER.Ptr(),
			Docker: &koyeb.DockerSource{
				Image: koyeb.PtrString("koyeb/github-runner"),
			},
			Regions: []string{region},
			InstanceTypes: []koyeb.DeploymentInstanceType{
				{Type: koyeb.PtrString(instanceType), Scopes: []string{fmt.Sprintf("region:%s", region)}},
			},
			Env: []koyeb.DeploymentEnv{
				{Key: koyeb.PtrString("REPO_URL"), Value: koyeb.PtrString(fmt.Sprintf("https://github.com/%s", payload.Repository.FullName))},
				{Key: koyeb.PtrString("GITHUB_TOKEN"), Value: koyeb.PtrString(api.GithubToken)},
				{Key: koyeb.PtrString("RUNNER_LABELS"), Value: koyeb.PtrString(fmt.Sprintf("koyeb-%s-%s", region, instanceType))},
			},
			Scalings: []koyeb.DeploymentScaling{
				{Min: koyeb.PtrInt64(1), Max: koyeb.PtrInt64(1), Scopes: []string{fmt.Sprintf("region:%s", region)}},
			},
		},
	}

	serviceId, err = api.KoyebAPIClient.CreateService(createService)
	if err != nil {
		return err
	}

	fmt.Printf("Created service %s\n", serviceId)
	return nil
}
