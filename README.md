# coredns traefic plugin

## Name

*traefik* - returns CNAMEs for hosts registered in Traefik instances

## Description

Extracts FQDN's from the `Host()` and `HostSNI()` values in traefik's http router rules (Traefik's `/api/http/routers` endpoint). Returns a CNAME result with the traefik instance's domain.

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

## Syntax

~~~ txt
traefik https://your-traefik.server.com/api {
  cname your-traefik.server.com
  refreshInterval 
  ttl 5
}
~~~

## Metrics

If monitoring is enabled (via the *prometheus* directive) the following metric is exported:

* `coredns_example_request_count_total{server}` - query count to the *example* plugin.

The `server` label indicated which server handled the request, see the *metrics* plugin for details.

## Ready

This plugin reports readiness to the ready plugin. It will be ready after the first refresh of data

## Examples

In this configuration, we forward all queries to 9.9.9.9 and print "example" whenever we receive
a query.

~~~ corefile
. {
  forward . 9.9.9.9
  example
}
~~~

Or without any external connectivity:

~~~ corefile
. {
  whoami
  example
}
~~~

## Also See

See the [manual](https://coredns.io/manual).
