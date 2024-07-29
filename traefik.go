package traefik

import (
	"context"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("traefik")

//goland:noinspection GoNameStartsWithPackageName
type TraefikConfigEntry struct {
	cname *string
	a     *[]net.IP
	ttl   uint32
}

//goland:noinspection GoNameStartsWithPackageName
type TraefikConfigEntryMap map[string]*TraefikConfigEntry

//goland:noinspection GoNameStartsWithPackageName
type TraefikConfig struct {
	baseUrl            *url.URL
	cname              *string
	a                  *[]net.IP
	ttl                uint32
	refreshInterval    uint32
	insecureSkipVerify bool
	hostMatcher        *regexp.Regexp
	apiHostname        string
	apiHostnameIsIp    bool
	apiIp              *net.IP
	resolveApiHost     bool
}

type Traefik struct {
	Next          plugin.Handler
	Config        *TraefikConfig
	TraefikClient *TraefikClient

	mappings TraefikConfigEntryMap
	ready    bool
	mutex    sync.RWMutex
	Fall     fall.F
}

func (t *Traefik) Name() string { return "traefik" }

func (t *Traefik) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	if state.QClass() != dns.ClassINET || state.QType() != dns.TypeA {
		log.Infof("Ignoring class %q, type %q", state.QClass(), state.QType())
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	qname := state.QName()
	answers := []dns.RR{}
	for _, q := range state.Req.Question {
		find := strings.ToLower(q.Name[:len(q.Name)-1])

		result := t.getEntry(find)
		if result != nil {
			if t.Config.cname != nil {
				answers = cname(qname, t.Config.ttl, t.Config.cname)
			} else if t.Config.a != nil {
				answers = a(qname, t.Config.ttl, *t.Config.a)
			}
		}
	}

	m := new(dns.Msg)
	if len(answers) == 0 {
		if t.Fall.Through(qname) && t.Next != nil {
			log.Debug("Falling through. 0 answers")
			return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
		}

		log.Debug("Returning NXDOMAIN")
		m.Rcode = dns.RcodeNameError
	}

	m.SetReply(r)
	m.Authoritative = true
	m.Answer = answers

	//goland:noinspection ALL
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (t *Traefik) start() error {
	log.Info("Starting!")
	err := t.refresh(true)

	if err != nil {
		log.Warningf("Failed to load Traefik HTTP routers, will retry: %s", err)
	}

	uptimeTicker := time.NewTicker(time.Duration(t.Config.refreshInterval) * time.Second)

	for {
		select {
		case <-uptimeTicker.C:
			log.Debug("Refreshing sites")
			err := t.refresh(false)
			if err != nil {
				log.Warningf("Error loading Traefik HTTP routers: %v", err)
			}
		}
	}
}

func (t *Traefik) getEntry(host string) *TraefikConfigEntry {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if t.Config.resolveApiHost && !t.Config.apiHostnameIsIp && t.Config.apiHostname == host {
		return &TraefikConfigEntry{
			a:   t.Config.a,
			ttl: t.Config.ttl,
		}
	}

	value, foundIt := t.mappings[host]
	if !foundIt {
		return nil
	}

	return value
}

func (t *Traefik) refresh(first bool) error {
	if first {
		log.Infof("Checking for Traefik HTTP routers...")
	}
	routers, err := t.TraefikClient.GetHttpRouters()
	if err != nil {
		log.Errorf("Error retrieving Traefik HTTP routers: %s", err)
		return err
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	adds, deletes := 0, 0
	fromTraefik := map[string]struct{}{}
	for _, s := range *routers {
		//goland:noinspection ALL
		strs := t.Config.hostMatcher.FindAllStringSubmatch(s.Rule, -1)
		for _, s := range strs {
			if len(s) == 3 {
				host := strings.ToLower(s[2])
				fromTraefik[host] = struct{}{}

				if t.Config.cname != nil {
					_, exists := t.mappings[host]
					if !exists {
						log.Infof("+ CNAME %s -> %s", host, *t.Config.cname)
						t.mappings[host] = &TraefikConfigEntry{
							cname: t.Config.cname,
							ttl:   t.Config.ttl,
						}
						adds += 1
					}
				} else if t.Config.a != nil {
					_, exists := t.mappings[host]

					if !exists {
						log.Infof("+ %s -> %s", host, ConvertIPsToString(*t.Config.a))
						t.mappings[host] = &TraefikConfigEntry{
							a:   t.Config.a,
							ttl: t.Config.ttl,
						}
						adds += 1
					}
				}
			}
		}
	}

	toDelete := map[string]struct{}{}
	for cachedHost := range t.mappings {
		_, stillExists := fromTraefik[cachedHost]
		if !stillExists {
			if t.Config.cname != nil {
				log.Infof("- CNAME %s -> %s", cachedHost, *t.Config.cname)
			} else if t.Config.a != nil {
				log.Infof("- A %s -> %s", cachedHost, ConvertIPsToString(*t.Config.a))
			}
			toDelete[cachedHost] = struct{}{}
			deletes += 1
		}
	}

	for del := range toDelete {
		delete(t.mappings, del)
	}

	if adds > 0 && deletes > 0 {
		log.Infof("Added %d, deleted %d entries", adds, deletes)
	} else if adds > 0 {
		log.Infof("Added %d entries", adds)
	} else if deletes > 0 {
		log.Infof("Deleted %d entries", deletes)
	} else {
		if first {
			log.Warning("Failed to load Traefik HTTP routes... Will try again")
		} else {
			log.Debug("No changes detected")
		}
	}

	t.ready = true
	return nil
}

func a(zone string, ttl uint32, ips []net.IP) []dns.RR {
	answers := make([]dns.RR, len(ips))
	for i, ip := range ips {
		r := new(dns.A)
		r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}
		r.A = ip
		answers[i] = r
	}
	return answers
}

func cname(zone string, ttl uint32, target *string) []dns.RR {
	answers := make([]dns.RR, 1)
	r := new(dns.CNAME)
	r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}
	r.Target = *target
	answers[0] = r
	return answers
}

func ConvertIPsToString(ips []net.IP) string {
	var ipStrings []string
	for _, ip := range ips {
		ipStrings = append(ipStrings, ip.String())
	}
	return strings.Join(ipStrings, ",")
}
