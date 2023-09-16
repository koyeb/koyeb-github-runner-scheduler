package koyeb_api

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/koyeb/koyeb-api-client-go/api/v1/koyeb"
)

type APIClient struct {
	ctx    context.Context
	client *koyeb.APIClient
}

func NewAPIClient(token string) APIClient {
	ctx := context.Background()
	ctx = context.WithValue(ctx, koyeb.ContextAccessToken, token)

	config := koyeb.NewConfiguration()
	config.Servers[0].URL = "https://app.koyeb.com"
	return APIClient{
		ctx:    ctx,
		client: koyeb.NewAPIClient(config),
	}
}

// Return the ID of a service named `name` in the application `appId`, or an empty string if the service does not exist.
func (api APIClient) GetService(appId string, name string) (string, error) {
	resp, _, err := api.client.ServicesApi.ListServices(api.ctx).AppId(appId).Name(name).Execute()
	if err != nil {
		return "", nil
	}
	services := resp.GetServices()
	if len(services) == 0 {
		return "", nil
	}
	return services[0].GetId(), nil
}

// Create or update a Koyeb application, and return the application ID.
func (api APIClient) UpsertApplication(name string) (string, bool, error) {
	listResp, resp, err := api.client.AppsApi.ListApps(api.ctx).Name(name).Execute()
	if err != nil {
		return "", false, errorFromHttpResponse(resp)
	}

	// Filtering on name returns all the applications that have the name as a prefix. Filter on the exact name.
	for _, app := range listResp.GetApps() {
		if app.GetName() == name {
			return app.GetId(), false, nil
		}
	}

	params := koyeb.NewCreateAppWithDefaults()
	params.SetName(name)
	createResp, resp, err := api.client.AppsApi.CreateApp(api.ctx).App(*params).Execute()
	if err != nil {
		return "", false, errorFromHttpResponse(resp)
	}
	return *createResp.GetApp().Id, true, nil
}

func (api APIClient) CreateService(createService koyeb.CreateService) (string, error) {
	res, resp, err := api.client.ServicesApi.CreateService(api.ctx).Service(createService).Execute()
	if err != nil {
		return "", errorFromHttpResponse(resp)
	}
	return res.Service.GetId(), nil
}

func errorFromHttpResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		body = []byte{}
	}
	return fmt.Errorf(
		"---\nHTTP/%s\n\n%s\n---\n",
		resp.Status,
		body,
	)
}
