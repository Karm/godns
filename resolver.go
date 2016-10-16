package main

import (
// Standard library packages
	"fmt"
	"strings"
	"sync"
	"time"
	"bytes"
//	"sort"
	"net"
	"strconv"
// Third party packages
	"github.com/miekg/dns"
	"os"
	"io/ioutil"
	"encoding/json"
	"net/http"
	"sync/atomic"
)

type Sinkhole struct {
	Sinkhole string `json:"sinkhole"`
}

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

type CoreError struct {
	When time.Time
	What string
}

func (e CoreError) Error() string {
	return fmt.Sprintf("%v: %v", e.When, e.What)
}

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, time.Duration(settings.Backend.HardRequestTimeout) * time.Millisecond)
}

var coreApiServer = "http://"+os.Getenv("SINKIT_CORE_SERVER")+":"+os.Getenv("SINKIT_CORE_SERVER_PORT")+"/sinkit/rest/blacklist/dns"
var transport = http.Transport{
	Dial: dialTimeout,
}
var coreDisabled uint32 = 1
var disabledSecondsTimestamp int64 = 0

func dryAPICall(query string, clientAddress string, qname string) {
	var trimmedQname = strings.TrimSuffix(qname, ".")
	if (atomic.LoadInt64(&disabledSecondsTimestamp) == 0) {
		logger.Debug("disabledSecondsTimestamp was 0, setting it to the current time")
		atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
		return
	}
	currentTime := int64(time.Now().Unix())
	lastStamp := atomic.LoadInt64(&disabledSecondsTimestamp)
	if ((currentTime - lastStamp)*1000 > settings.Backend.SleepWhenDisabled) {
		logger.Debug("Doing dry API call...")
		start := time.Now()
		//Doesn't hurt IP
		_, err := doAPICall(trimmedQname, clientAddress, trimmedQname)
		elapsed := time.Since(start)
		if (err != nil) {
			logger.Info("Core remains DISABLED. Gonna wait. Error: %s", err)
			atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
			return
		}
		if (elapsed > time.Duration(settings.Backend.FitResponseTime)*time.Millisecond) {
			logger.Info("Core remains DISABLED. Gonna wait. Elapsed time: %s, FitResponseTime: %s", elapsed, time.Duration(settings.Backend.FitResponseTime)*time.Millisecond)
			atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
			return
		}
		logger.Debug("Core is now ENABLED")
		atomic.StoreUint32(&coreDisabled, 0)
	} else {
		logger.Debug("Not enough time passed, waiting for another call. Elapsed: %s ms, Limit: %s ms", (currentTime - lastStamp)*1000, settings.Backend.SleepWhenDisabled)
	}
	return
}

func doAPICall(query string, clientAddress string, trimmedQname string) (value bool, err error) {
	var bufferQuery bytes.Buffer
	bufferQuery.WriteString(coreApiServer)
	bufferQuery.WriteString("/")
	bufferQuery.WriteString(clientAddress)
	bufferQuery.WriteString("/")
	bufferQuery.WriteString(query)
	bufferQuery.WriteString("/")
	bufferQuery.WriteString(trimmedQname)
	url := bufferQuery.String()
	logger.Debug("URL:>", url)

	//var jsonStr = []byte(`{"title":"Buy cheese and bread for breakfast."}`)
	//req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonStr))
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("X-sinkit-token", os.Getenv("SINKIT_ACCESS_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.Debug("There has been an error with backend.")
		return false, err
	}
	defer resp.Body.Close()

	logger.Debug("Response Status:", resp.Status)
	logger.Debug("Response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	if (resp.StatusCode != 200) {
		logger.Debug("Response Body:", string(body))
		return false, CoreError{time.Now(), "Non HTTP 200."}
	}
	// i.e. "null" or possible stray byte, not a sinkhole IP
	if (len(body) < 6) {
		logger.Debug("Response short.")
		return false, nil
	}

	var sinkhole Sinkhole
	err = json.Unmarshal(body, &sinkhole)
	if err != nil {
		logger.Debug("There has been an error with unmarshalling the response: %s", body)
		return false, err
	}
	fmt.Printf("\nSINKHOLE RETURNED from Core[%s]\n", sinkhole.Sinkhole)

	return true, nil
}

func sinkitBackendCall(query string, clientAddress string, trimmedQname string) (bool) {
	//TODO This is just a provisional check. We need to think it over...
	if (len(query) > 250) {
		fmt.Printf("Query is too long: %d\n", len(query))
		return false
	}
	if (len(clientAddress) < 3) {
		fmt.Printf("Client address is too short: %s\n", clientAddress)
		return false
	}
	if (len(trimmedQname) < 3 || len(trimmedQname) > 250) {
		fmt.Printf("Query FQDN is likely invalid: %s\n", trimmedQname)
		return false
	}

	start := time.Now()
	goToSinkhole, err := doAPICall(query, clientAddress, trimmedQname)
	elapsed := time.Since(start)
	if (err != nil) {
		atomic.StoreUint32(&coreDisabled, 1)
		atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
		logger.Info("Core was DISABLED. Error: %s", err)
		return false
	}
	if (elapsed > time.Duration(settings.Backend.FitResponseTime)*time.Millisecond) {
		atomic.StoreUint32(&coreDisabled, 1)
		atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
		logger.Info("Core was DISABLED. Elapsed time: %s, FitResponseTime: %s", elapsed, time.Duration(settings.Backend.FitResponseTime)*time.Millisecond)
		return false
	}

	return goToSinkhole
}

// Dummy playground
func sinkByHostname(qname string, clientAddress string) (bool) {
	var trimmedQname = strings.TrimSuffix(qname, ".")
	// Yes, twice trimmedQname
	return sinkitBackendCall(trimmedQname, clientAddress, trimmedQname)
}

// We do not sinkhole here, the side effect is that CNAMEs slip through.
func sinkByIPAddress(msg *dns.Msg, clientAddress string, qname string) {
	var trimmedQname = strings.TrimSuffix(qname, ".")
	for _, element := range msg.Answer {
		logger.Debug("\nKARMTAG: RR Element: %s\n", element)
		vals := strings.Split(element.String(), "	")
		// We loop through the elements, TTL, IN, Class...
		for i := range vals {
			logger.Debug("KARMTAG: value: %s\n", vals[i])
			if (strings.EqualFold(vals[i], "A") || strings.EqualFold(vals[i], "CNAME") || strings.EqualFold(vals[i], "AAAA")) {
				logger.Debug("KARMTAG: value matches: %s\n", vals[i])

				// Length in bytes, not runes. Shorter doesn't make sense.
				// We ditch .root-servers.net.
				if (len(vals) > i+1 && len(vals[i+1]) > 3 && !strings.HasSuffix(vals[i+1], ".root-servers.net.")) {
					logger.Debug("KARMTAG: to send to Core: %s\n", vals[i+1])
					go sinkitBackendCall(strings.TrimSuffix(vals[i+1], "."), clientAddress, trimmedQname)
				}
				break
			}
		}
	}
}

// Move this to configuration
var infDisabled, infDisabledErr = strconv.ParseBool(os.Getenv("SINKIT_RESOLVER_DISABLE_INFINISPAN"))
func processCoreCom(msg *dns.Msg, qname string, clientAddress string) {
	// Don't bother contacting Infinispan Sinkit Core
	if (infDisabledErr == nil && infDisabled) {
		logger.Debug("SINKIT_RESOLVER_DISABLE_INFINISPAN TRUE\n")
		return
	} else {
		logger.Debug("SINKIT_RESOLVER_DISABLE_INFINISPAN FALSE or N/A\n")
	}
	logger.Debug("\n KARMTAG: Resolved to: %s\n", msg.Answer)
	if (atomic.LoadUint32(&coreDisabled) == 1) {
		logger.Debug("Core is DISABLED. Gonna call dryAPICall.")
		//TODO qname or r for the dry run???
		go dryAPICall(qname, clientAddress, qname)
		logger.Debug("...returning.")
	} else {
		sinkByIPAddress(msg, clientAddress, qname)
		// We do not sinkhole based on IP address.
		if (sinkByHostname(qname, clientAddress)) {
			logger.Debug("\n KARMTAG: %s GOES TO SINKHOLE!\n", msg.Answer)
			sendToSinkhole(msg, qname)
		}
	}
}

// Dummy playground
func sendToSinkhole(msg *dns.Msg, qname string) {
	var buffer bytes.Buffer
	buffer.WriteString(qname)
	buffer.WriteString("	")
	buffer.WriteString("10	")
	buffer.WriteString("IN	")
	buffer.WriteString("A	")
	buffer.WriteString(os.Getenv("SINKIT_SINKHOLE_IP"))
	//msg.Answer[0], _ = dns.NewRR(buffer.String())
	//Sink all records:
	sinkRecord, _ := dns.NewRR(buffer.String())
	msg.Answer = []dns.RR{sinkRecord}
	//Debug("\n KARMTAG: A record: %s", msg.Answer[0].(*dns.A).String())
	//Debug("\n KARMTAG: CNAME record: %s", msg.Answer[1].(*dns.CNAME).String())
	return
}

// Lookup will ask each nameserver in top-to-bottom fashion, starting a new request
// in every second, and return as early as possible (have an answer).
// It returns an error if no request has succeeded.
func (r *Resolver) Lookup(net string, req *dns.Msg, remoteAddress net.Addr) (message *dns.Msg, err error) {
	c := &dns.Client{
		Net:          net,
		ReadTimeout:  r.Timeout(),
		WriteTimeout: r.Timeout(),
	}

	qname := req.Question[0].Name
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
			logger.Warn("%s failed to get an valid answer on %s", qname, nameserver)
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

	ticker := time.NewTicker(time.Duration(settings.ResolvConfig.Interval) * time.Millisecond)
	defer ticker.Stop()

	// Start lookup on each nameserver top-down, in every second
	for _, nameserver := range r.Nameservers() {
		wg.Add(1)
		go L(nameserver)
		// but exit early, if we have an answer
		select {
		case r := <-res:
			processCoreCom(r, qname, clientAddress)
			return r, nil
		case <-ticker.C:
			continue
		}
	}
	// wait for all the namservers to finish
	wg.Wait()

	select {
	case r := <-res:
	//TODO: Redundant?
		processCoreCom(r, qname, clientAddress)
		return r, nil
	default:
		return nil, ResolvError{qname, net, r.Nameservers()}
	}
}

// Namservers return the array of nameservers, with port number appended.
// '#' in the name is treated as port separator, as with dnsmasq.
func (r *Resolver) Nameservers() (ns []string) {
	if (settings.Backend.UseExclusively) {
		logger.Debug("Using exclusively these backend servers:\n")
		for _, server := range settings.Backend.BackendResolvers {
			logger.Debug(" Appending backend server: %s \n", server)
			ns = append(ns, server)
		}
	} else {
		for _, server := range r.config.Servers {
			if i := strings.IndexByte(server, '#'); i > 0 {
				server = server[:i] + ":" + server[i+1:]
			} else {
				server = server + ":" + r.config.Port
			}
			ns = append(ns, server)
		}
	}
	return
}

func (r *Resolver) Timeout() time.Duration {
	return time.Duration(r.config.Timeout) * time.Second
}
