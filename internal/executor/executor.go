package executor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type API struct {
	APISecret   string
	GithubToken string
	KoyebToken  string
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
	router.HandleFunc("/", api.executor)

	fmt.Printf("Start listening on %d\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

func (api API) executor(w http.ResponseWriter, r *http.Request) {
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
		api.startRunner(&payload)
		w.WriteHeader(http.StatusOK)
	case "completed":
		fmt.Printf(">> Received GitHub Action: %s/%s\n", payload.WorkflowJob.WorkflowName, payload.Action)
		api.stopRunner(&payload)
		w.WriteHeader(http.StatusOK)
	default:
		fmt.Printf(">> Ignoring GitHub action: %s/%s\n", payload.WorkflowJob.WorkflowName, payload.Action)
		w.WriteHeader(http.StatusOK)
	}
}

func (api API) startRunner(payload *WebHookPayload) {
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
		fmt.Printf("%s/%s does not have a label targeting this executor, ignoring\n", payload.WorkflowJob.WorkflowName, payload.Action)
		return
	}

	fmt.Printf("Staring %s runner on %s\n", instanceType, region)

	// Create the Koyeb application. The CLI returns an error if the application already exists, so we do not check the error returned by cmd.Run().
	cmd := exec.Command("koyeb", "--token", api.KoyebToken, "app", "create", "github-runner")
	cmd.Run()

	serviceName := fmt.Sprintf("runner-%d", payload.WorkflowJob.RunId)
	fmt.Printf("Creating Koyeb service github-runner/%s\n", serviceName)
	cmd = exec.Command(
		"koyeb",
		"service",
		"create",
		"--app", "github-runner",
		"--type", "worker",
		"--docker", "koyeb/github-runner",
		"--region", region,
		"--instance-type", instanceType,
		"--env", fmt.Sprintf("REPO_URL=https://github.com/%s", payload.Repository.FullName),
		"--env", fmt.Sprintf("GITHUB_TOKEN=%s", api.GithubToken),
		"--env", fmt.Sprintf("RUNNER_LABELS=koyeb-%s-%s", region, instanceType),
		"--env", "EPHEMERAL=true",
		serviceName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!! Error while creating the service. The Koyeb CLI returned the following message:\n---%s\n---\n", output)
	}
}

func (api API) stopRunner(payload *WebHookPayload) {
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
		fmt.Printf("%s/%s does not have a label targeting this executor, ignoring\n", payload.WorkflowJob.WorkflowName, payload.Action)
		return
	}

	serviceName := fmt.Sprintf("runner-%d", payload.WorkflowJob.RunId)
	fmt.Printf("Removing runner %s\n", serviceName)

	cmd := exec.Command(
		"koyeb",
		"service",
		"delete",
		fmt.Sprintf("github-runner/%s", serviceName),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "!!! Error while creating the service. The Koyeb CLI returned the following message:\n---%s\n---\n", output)
	}
}
