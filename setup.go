package traefik

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/coredns/caddy"
)

const defaultTraefikApiEndpoint = "https://traefik.example.com/api"
const defaultTraefikCname = "traefik.example.com"
const defaultTtl uint32 = 30
const defaultRefreshInterval uint32 = 30

func init() {
	caddy.RegisterPlugin("traefik", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func createPlugin(c *caddy.Controller) (*Traefik, error) {
	hostMatcher := regexp.MustCompile(`Host(SNI)?\(` + "`([^`]+)`" + `\)`)

	cfg := &TraefikConfig{
		cname:           defaultTraefikCname,
		hostMatcher:     hostMatcher,
		ttl:             defaultTtl,
		refreshInterval: defaultRefreshInterval,
	}

	traefik := &Traefik{
		Config: cfg,
	}

	defaultBaseUrl, err := url.Parse(defaultTraefikApiEndpoint)
	if err != nil {
		return traefik, err
	}

	cfg.baseUrl = defaultBaseUrl

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 1 {
			baseUrl, err := url.Parse(args[0])
			if err != nil {
				return traefik, err
			}

			cfg.baseUrl = baseUrl
		}

		if len(args) > 1 {
			return traefik, c.ArgErr()
		}

		for c.NextBlock() {
			var value = c.Val()
			switch value {
			case "cname":
				if !c.NextArg() {
					return traefik, c.ArgErr()
				}
				cfg.cname, _ = strings.CutSuffix(c.Val(), ".")
			case "refreshInterval":
				if !c.NextArg() {
					return traefik, c.ArgErr()
				}
				refreshInterval, err := strconv.ParseUint(c.Val(), 10, 32)
				if err != nil {
					return traefik, err
				}
				if refreshInterval > 0 {
					cfg.refreshInterval = uint32(refreshInterval)
				}
			case "ttl":
				if !c.NextArg() {
					return traefik, c.ArgErr()
				}
				ttl, err := strconv.ParseUint(c.Val(), 10, 32)
				if err != nil {
					return traefik, err
				}
				if ttl > 0 {
					cfg.ttl = uint32(ttl)
				}
			default:
				return traefik, c.Errf("unknown property: '%s'", c.Val())
			}
		}
	}

	traefikClient, err := NewTraefikClient(cfg)
	if err != nil {
		return nil, err
	}

	traefik.TraefikClient = traefikClient
	traefik.mappings = make(TraefikConfigEntryMap)

	return traefik, nil
}

func setup(c *caddy.Controller) error {
	traefik, err := createPlugin(c)
	if err != nil {
		return err
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		traefik.Next = next
		return traefik
	})

	go traefik.start()

	return nil
}
