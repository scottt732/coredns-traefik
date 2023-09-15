package traefik

import (
	"encoding/json"
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

func NewTraefikClient(cfg *TraefikConfig, options ...func(*http.Client) error) (*TraefikClient, error) {
	httpClient := &http.Client{}
	for _, op := range options {
		err := op(httpClient)
		if err != nil {
			return nil, err
		}
	}

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

	if response.StatusCode != 200 {
		log.Errorf("Received invalid response from API: %d", response.StatusCode)
		return nil, err
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	body, err := io.ReadAll(response.Body)

	str := string(body[:])
	log.Debug(str)

	if err != nil {
		log.Errorf("Failed to read http routers response: %q", err)
		return nil, err
	}

	result := []HttpRouter{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Errorf("Failed to parse json body: %q", err)
		return nil, err
	}

	return &result, nil
}
