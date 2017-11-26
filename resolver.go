package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"net"
	"github.com/miekg/dns"
)

type ResolvError struct {
	qname, net  string
	nameservers []string
}

func (e ResolvError) Error() string {
	errmsg := fmt.Sprintf("%s resolv failed on %s (%s)", e.qname, strings.Join(e.nameservers, "; "), e.net)
	return errmsg
}

type Resolver struct {
	config *dns.ClientConfig
}

func (r *Resolver) Timeout() time.Duration {
	return time.Duration(r.config.Timeout) * time.Millisecond
}

var nameservers []string

// Lookup will ask each nameserver in top-to-bottom fashion, starting a new request
// in every second, and return as early as possible (have an answer).
// It returns an error if no request has succeeded.
func (r *Resolver) Lookup(net string, req *dns.Msg, remoteAddress net.Addr, oraculumCache Cache, caches *ListCache) (message *dns.Msg, err error) {

	if(settings.BACKEND_RESOLVERS_TCP_ONLY) {
		net = "tcp"
	}

	c := &dns.Client{
		Net:          net,
		ReadTimeout:  r.Timeout(),
		WriteTimeout: r.Timeout(),
	}

	/** EDNS(0) was causing problems in large packets - playstation, meteo, ... removing
	if net == "udp" {
		req = req.SetEdns0(uint16(settings.GODNS_UDP_PACKET_SIZE), true)
	}
	*/

	qname := req.Question[0].Name
	//TODO: IPv6
	clientAddress := strings.Split(remoteAddress.String(), ":")[0]

	res := make(chan *dns.Msg, 1)
	var wg sync.WaitGroup
	L := func(nameserver string) {
		defer wg.Done()
		r, rtt, err := c.Exchange(req, nameserver)
		if err != nil {
			logger.Warn("%s socket error on %s", qname, nameserver)
			logger.Warn("error:%s", err.Error())
			return
		}
		// If SERVFAIL happen, should return immediately and try another upstream resolver.
		// However, other Error code like NXDOMAIN is an clear response stating
		// that it has been verified no such domain exists and ask other resolvers
		// would make no sense. See more about #20
		if r != nil && r.Rcode != dns.RcodeSuccess {
			logger.Debug("%s failed to get an valid answer on %s", qname, nameserver)
			if r.Rcode == dns.RcodeServerFailure {
				return
			}
		} else {
			logger.Debug("\n KARMTAG: %s resolv on %s (%s) ttl: %d", UnFqdn(qname), nameserver, net, rtt)
		}
		select {
		case res <- r:
		default:
		}
	}

	ticker := time.NewTicker(time.Duration(settings.BACKEND_RESOLVER_TICK) * time.Millisecond)
	defer ticker.Stop()
	// Start lookup on each nameserver top-down, in every second
	for _, nameserver := range nameservers {
		wg.Add(1)
		go L(nameserver)
		// but exit early, if we have an answer
		select {
		case r := <-res:
			processCoreCom(r, qname, clientAddress, oraculumCache, caches)
			return r, nil
		case <-ticker.C:
			continue
		}
	}
	// wait for all the namservers to finish
	wg.Wait()
	select {
	case r := <-res:
		processCoreCom(r, qname, clientAddress, oraculumCache, caches)
		return r, nil
	default:
		return nil, ResolvError{qname, net, nameservers}
	}
}

func (r *Resolver) init() {
	nameservers = r.Nameservers()
}

// Namservers return the array of nameservers, with port number appended.
// '#' in the name is treated as port separator, as with dnsmasq.
func (r *Resolver) Nameservers() (ns []string) {
	if (settings.BACKEND_RESOLVERS_EXCLUSIVELY) {
		logger.Debug("Using exclusively these backend servers:\n")
		for _, server := range settings.BACKEND_RESOLVERS {
			logger.Debug(" Appending backend server: %s \n", server)
			ns = append(ns, server)
		}
	} else {
		for _, server := range settings.BACKEND_RESOLVERS {
			logger.Debug(" Appending backend server: %s \n", server)
			ns = append(ns, server)
		}
		for _, server := range r.config.Servers {
			if i := strings.IndexByte(server, '#'); i > 0 {
				server = server[:i] + ":" + server[i+1:]
			} else {
				server = server + ":" + r.config.Port
			}
			logger.Debug(" Appending backend server: %s \n", server)
			ns = append(ns, server)
		}
	}
	return
}
