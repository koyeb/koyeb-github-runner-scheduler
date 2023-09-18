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

// GetService retrieves the ID of the service named `name` in the application `appId`, or an empty string if the service does not exist.
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

// UpsertApplication creates or updates a Koyeb application, and returns the application ID.
func (api APIClient) UpsertApplication(name string) (string, bool, error) {
	appId, err := api.GetApp(name)
	if err != nil {
		return "", false, err
	}
	// Application already exists
	if appId != "" {
		return appId, false, nil
	}

	params := koyeb.NewCreateAppWithDefaults()
	params.SetName(name)
	createResp, resp, err := api.client.AppsApi.CreateApp(api.ctx).App(*params).Execute()
	if err != nil {
		return "", false, errorFromHttpResponse(resp)
	}
	return *createResp.GetApp().Id, true, nil
}

// CreateService performs a Koyeb API call to create a service.
func (api APIClient) CreateService(createService koyeb.CreateService) (string, error) {
	res, resp, err := api.client.ServicesApi.CreateService(api.ctx).Service(createService).Execute()
	if err != nil {
		return "", errorFromHttpResponse(resp)
	}
	return res.Service.GetId(), nil
}

// GetApp returns the Koyeb application named `name`, or an empty string if the application does not exist.
func (api APIClient) GetApp(name string) (string, error) {
	res, resp, err := api.client.AppsApi.ListApps(api.ctx).Name(name).Execute()
	if err != nil {
		return "", errorFromHttpResponse(resp)
	}

	// Filtering on name returns all the applications that have the name as a prefix. Filter on the exact name.
	for _, app := range res.GetApps() {
		if app.GetName() == name {
			return app.GetId(), nil
		}
	}
	return "", nil
}

// ListServices returns all the services in the application `appId`.
func (api APIClient) ListServices(appId string) ([]string, error) {
	res, resp, err := api.client.ServicesApi.ListServices(api.ctx).AppId(appId).Execute()
	if err != nil {
		return nil, errorFromHttpResponse(resp)
	}

	services := make([]string, 0, len(res.GetServices()))
	for _, svc := range res.GetServices() {
		services = append(services, svc.GetId())
	}
	return services, nil
}

// DeleteService performs a Koyeb API call to delete a service. Do not fail if the service does not exist.
func (api APIClient) DeleteService(serviceId string) (bool, error) {
	_, resp, err := api.client.ServicesApi.DeleteService(api.ctx, serviceId).Execute()
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, errorFromHttpResponse(resp)
	}
	return true, nil
}

// Consume the response body to format a meaningful error message from an error HTTP response.
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
