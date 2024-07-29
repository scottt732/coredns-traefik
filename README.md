# coredns traefik plugin

## Name

*traefik* - returns CNAMEs for hosts registered in Traefik instances

## Description

Extracts FQDN's from the `Host()` and `HostSNI()` values in traefik's http router rules (Traefik's `/api/http/routers` endpoint). Returns a `CNAME` result with the traefik instance's domain.

### Why?

Homelab + free time. Prior to using traefik, I had a few nginx containers running in different vlans in my homelab. 
I would create config files by hand, setup static DNS entries, generate certs, and reverse proxy to docker 
containers by hand. 

Traefik took care of a lot of that for me, but I still needed to setup the static DNS entries. Basically I would 
have a single IP for traefik and a bunch of CNAMEs to it for the various containerized services. Rather than go 
into pfSense to do this every time, I figured I'd find something more automatic. So I spun up CoreDNS and threw 
this plugin together to poll the Traefik API periodically and figure out what host names I have http routers 
referring to. For each of those, I can respond with a CNAME to the Traefik server on-the-fly. 

When you pair this with Traefik's ability to configure routes and services via Docker labels, this completely 
automates the DNS, TLS, and reverse proxy pain when spinning up new containers. It's a nice stopgap until I get 
around to making the switch to kubernetes & should play nicely during the transition period.

## Compilation

This package will always be compiled as part of CoreDNS and not in a standalone way. It will require you to use `go get` or as a dependency on [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg).

The [manual](https://coredns.io/manual/toc/#what-is-coredns) will have more information about how to configure and extend the server with external plugins.

A simple way to consume this plugin, is by adding the following on [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg), and recompile it as [detailed on coredns.io](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-compile-time-configuration-file).

~~~
traefik:github.com/scottt732/coredns-traefik
~~~

Put this early in the plugin list, so that *traefik* is executed before any of the other plugins.

After this you can compile coredns by:

``` sh
go generate
go build
```

Or you can instead use make:

``` sh
make
```

## Settings

~~~ txt
traefik https://your-traefik.homelab.net/api {
  cname your-traefik.homelab.net
  a 10.0.0.2
  refreshinterval 30
  ttl 30
  insecureskipverify [true|false*]
  resolveapihost [true|false*]
}
~~~

- `https://your-traefik.homelab.net/api` - This is the endpoint that your Traefik API is listening on. You may need to configure Traefik to expose this.
- Either (but not both):
  - `cname` - The host name to return for HTTP routes defined in Traefik. Typically, this is the same host name as in the API URI above.
  - `a` - The IP address to return for HTTP routes defined in Traefik. Typically, this is the IP address of the Traefik server.
- `refreshinterval` - How frequently to poll for changes in Traefik
- `ttl` - How long should the results be persisted in downstream DNS caches
- `insecureskipverify` - Optional. if your Traefik API endpoint requires HTTPS but you don't have a valid/trusted certificate
- `resolveapihost` - Optional. When true and you've chosen `a` above, this will cause the plugin to resolve `your-traefik.homelab.net` itself to the IP address specified in the `a` block  
- `fallthrough` - Optional 

## Syntax

### Option 1: Returning CNAMEs

~~~ txt
traefik https://your-traefik.homelab.net/api {
  cname your-traefik.homelab.net
  refreshinterval 30
  ttl 30
}
~~~

- `https://your-traefik.homelab.net/api` refers to the base [Traefik API endpoint](https://doc.traefik.io/traefik/operations/api/) (without trailing slash). Given the base URL, this will hit `https://your-traefik.homelab.net/api/http/routers` endpoint.
- `cname` is the fully qualified domain name (with or without trailing `.`) that matching requests will have returned. Usually this will be the host name of the API endpoint above.
- `refreshinterval` specifies how frequently that `api/http/routers` endpoint is polled for changes (in seconds)
- `ttl` determines that time-to-live for successful responses (in seconds).

### Option 2: Returning A records 

~~~ txt
traefik https://your-traefik.homelab.net/api {
  a 10.0.0.2
  refreshinterval 30
  ttl 30
}
~~~

- `a` indicates the IP address(es) of `your-traefik.homelab.net`. This IP address will be returned as `A` records for all HTTP Routers discovered in Traefik.
- You can optionally specify a `resolveapihost [true|false] (optional, default=false)` to have this plugin return `A 10.0.0.2` for `your-traefik.homelab.net` lookups

### Example

Imagine you have services in your homelab which you'd like to expose internally/externally as `homelab.net` subdomains.

- Traefik is listening on 80/443 on a host whose IP is `10.10.10.2`
- The primary gateway/DNS is on `10.10.10.1`
- You want `traefik.homelab.net` to route to Traefik's dashboard/API.
- You want a container named `gitea` to be resolvable as `mytestservice.homelab.net` with Traefik taking care of TLS/acme and reverse proxying.

Given this CoreDNS corefile fragment:

```
homelab.net:53 {
    hosts {
        10.10.10.2 traefik.homelab.net
        fallthrough
    }
    traefik https://traefik.homelab.net/api {
        cname traefik.homelab.net
        refreshinterval 30
        ttl 5
    }
    forward . dns://10.10.10.1
}
```

This will cause requests for `traefik.homelab.net` to return the A record 10.10.0.2.

Then we can use a compose.yml like this.

```
...
services:
  gitea:
    image: gitea/gitea:latest
    container_name: gitea
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.gitea-web.rule=Host(`gitea.homelab.net`)"
      - "traefik.http.routers.gitea-web.service=gitea-web-svc"
      - "traefik.http.routers.gitea-web.entrypoints=websecure"
      - "traefik.http.routers.gitea-web.tls=true"
      - "traefik.http.routers.gitea-web.tls.domains[0].main=homelab.net"
      - "traefik.http.routers.gitea-web.tls.domains[0].sans=*.homelab.net"
      - "traefik.http.services.gitea-web-svc.loadbalancer.server.scheme=http"
      - "traefik.http.services.gitea-web-svc.loadbalancer.server.port=3000"
    ...
```

The container labels cause Traefik's routers & services to get wired together, waiting for a match 
on `gitea.homelab.net`. 

This plugin polls the Traefik API and discovers `gitea.homelab.net`. It dynamically returns a CNAME 
to `traefik.homelab.net`, which gets resolved via the A record to `10.10.10.2`. Traefik receives the
request and sees the host name matches its rule.

In my case, pfSense's DNS Resolver handles DNS for my network and I setup a conditional forwarder for 
the zone `homelab.net` to point just those requests to the CoreDNS instance. Alternatively, you can 
send all of your DNS requests through CoreDNS.

## Metrics

If monitoring is enabled (via the *prometheus* directive) the following metric is exported:

* `coredns_traefik_request_count_total{server}` - query count to the *traefik* plugin.

The `server` label indicated which server handled the request, see the *metrics* plugin for details.

## Ready

This plugin reports readiness to the ready plugin. It will be ready after the first refresh of data

## Also See

See the [manual](https://coredns.io/manual).
