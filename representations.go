package traefik

type HttpRouterTlsDomain struct {
	Main string   `json:"main"`
	Sans []string `json:"sans"`
}

type HttpRouterTls struct {
	Options string                `json:"options"`
	Domains []HttpRouterTlsDomain `json:"domains"`
}

type HttpRouter struct {
	EntryPoints []string       `json:"entryPoints"`
	Service     string         `json:"service"`
	Rule        string         `json:"rule"`
	Priority    int            `json:"priority"`
	Tls         *HttpRouterTls `json:"tls,omitempty"`
	Status      string         `json:"status"`
	Using       []string       `json:"using"`
	Name        string         `json:"name"`
	Provider    string         `json:"provider"`
}
