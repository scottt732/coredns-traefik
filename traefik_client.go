package traefik

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type ITraefikClient interface {
	GetHttpRouters() (*[]HttpRouter, error)
}

type TraefikClient struct {
	ITraefikClient
	httpRoutersUrl string
	config         *TraefikConfig
	client         *http.Client
}

func NewTraefikClient(cfg *TraefikConfig) (*TraefikClient, error) {
	httpClient := &http.Client{}

	httpRoutersUrl, err := url.JoinPath(cfg.baseUrl.String(), "/http/routers")
	if err != nil {
		return nil, err
	}

	client := &TraefikClient{
		httpRoutersUrl: httpRoutersUrl,
		client:         httpClient,
		config:         cfg,
	}

	return client, nil
}

func (c *TraefikClient) GetHttpRouters() (*[]HttpRouter, error) {
	log.Debugf("Connecting to %s", c.httpRoutersUrl)
	response, err := c.client.Get(c.httpRoutersUrl)

	if err != nil {
		log.Errorf("Failed to fetch http routers: %q", err)
		return nil, err
	}

	var body []byte
	var readErr error
	if response.Body != nil {
		defer response.Body.Close()
		body, readErr = io.ReadAll(response.Body)
	}

	if readErr != nil {
		log.Errorf("Failed to read response body: %q", readErr)
		return nil, readErr
	}

	if response.StatusCode != 200 {
		if body != nil {
			bodyStr := string(body[:])
			return nil, fmt.Errorf("Received %d response from API: %s", response.StatusCode, bodyStr)
		} else {
			return nil, fmt.Errorf("Received %d response from API", response.StatusCode)
		}
	}

	result := []HttpRouter{}

	if body != nil {
		err = json.Unmarshal(body, &result)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse json body: %q", err)
		}
	}

	return &result, nil
}
