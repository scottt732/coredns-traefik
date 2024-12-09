package traefik

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/coredns/caddy"
)

const defaultTraefikApiEndpoint = "https://traefik.example.com/api"
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
		cname:              nil,
		a:                  nil,
		hostMatcher:        hostMatcher,
		ttl:                defaultTtl,
		refreshInterval:    defaultRefreshInterval,
		insecureSkipVerify: false,
		resolveApiHost:     false,
	}

	traefik := &Traefik{
		Config: cfg,
	}

	defaultBaseUrl, err := url.Parse(defaultTraefikApiEndpoint)
	if err != nil {
		return traefik, err
	}

	cfg.baseUrl = defaultBaseUrl

	mode := -1
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 1 {
			baseUrl, err := url.Parse(args[0])
			if err != nil {
				return traefik, err
			}

			cfg.baseUrl = baseUrl
		}

		apiHostname := cfg.baseUrl.Hostname()
		cfg.apiHostname = apiHostname

		apiIp := net.ParseIP(apiHostname)
		cfg.apiHostnameIsIp = apiIp != nil
		if apiIp != nil {
			cfg.apiIp = &apiIp
		}

		if len(args) > 1 {
			return traefik, c.ArgErr()
		}

		for c.NextBlock() {
			var value = c.Val()
			//goland:noinspection SpellCheckingInspection
			switch value {
			case "cname":
				if !c.NextArg() {
					return traefik, c.ArgErr()
				}
				val := c.Val()
				cfg.cname = &val
				if mode == 1 {
					return traefik, c.Err("traefik cname and a are mutually exclusive")
				}
				mode = 0
			case "a":
				val := c.RemainingArgs()
				ips, err := convertStringsToIPs(val)
				if err != nil {
					return traefik, c.Errf("traefik config failed to parse ip addresses: %s", err)
				}
				cfg.a = &ips
				if len(ips) == 0 {
					return traefik, c.ArgErr()
				}
				if mode == 0 {
					return traefik, c.Err("traefik config cname and a are mutually exclusive")
				}
				mode = 1
			case "refreshinterval":
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
			case "insecureskipverify":
				if c.NextArg() {
					cfg.insecureSkipVerify = parseBoolWithDefault(c.Val(), false)
				} else {
					cfg.insecureSkipVerify = true
				}
			case "fallthrough":
				traefik.Fall.SetZonesFromArgs(c.RemainingArgs())
			case "resolveapihost":
				if c.NextArg() {
					cfg.resolveApiHost = parseBoolWithDefault(c.Val(), false)
				} else {
					cfg.resolveApiHost = true
				}
			default:
				return traefik, c.Errf("unknown property: '%s'", c.Val())
			}
		}
	}

	if mode == -1 {
		return traefik, c.Errf("traefik config requires a cname or a")
	}

	traefikClient, err := NewTraefikClient(cfg)
	if err != nil {
		return nil, err
	}

	traefik.TraefikClient = traefikClient
	traefik.mappings = make(TraefikConfigEntryMap)

	log.Infof("base url ............ %s", cfg.baseUrl)
	log.Infof("cname ............... %v", cfg.cname)
	log.Infof("a ................... %v", cfg.a)
	log.Infof("ttl ................. %v", cfg.ttl)
	log.Infof("refreshInterval ..... %v", cfg.refreshInterval)
	log.Infof("insecureSkipVerify .. %v", cfg.insecureSkipVerify)
	log.Infof("apiHostname ......... %v", cfg.apiHostname)
	log.Infof("apiIp ............... %v", cfg.apiIp)
	log.Infof("resolveApiHost ...... %v", cfg.resolveApiHost)

	return traefik, nil
}

func convertStringsToIPs(ips []string) ([]net.IP, error) {
	var result []net.IP
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("invalid IPv4 address: %s", ipStr)
		}
		result = append(result, ip.To4())
	}
	return result, nil
}

func parseBoolWithDefault(str string, defaultValue bool) bool {
	parsedValue, err := strconv.ParseBool(str)
	if err != nil {
		log.Warningf("Failed to parse %s as bool. Defaulting to %s", str, defaultValue)
		return defaultValue
	}
	return parsedValue
}

func setup(c *caddy.Controller) error {
	traefik, err := createPlugin(c)
	if err != nil {
		return err
	}

	go traefik.start()

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		traefik.Next = next
		return traefik
	})

	return nil
}
