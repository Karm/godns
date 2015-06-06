package main

import (
// Standard library packages
	"fmt"
	"strings"
	"sync"
	"time"
	"bytes"
//	"sort"
	"net/http"
// Third party packages
	"github.com/miekg/dns"
	"os"
	"io/ioutil"
	"encoding/json"
)

type Listed struct {
	Year uint16 `json:"year"`
	Month uint8 `json:"month"`
	DayOfMonth uint8 `json:"dayOfMonth"`
	HourOfDay  uint8 `json:"hourOfDay"`
	Minute uint8 `json:"minute"`
	Second  uint8 `json:"second"`
}

type BlackListedRecord struct {
	Source string `json:"source"`
	Listed Listed `json:"listed"`
	BlackListedDomainOrIP string `json:"blackListedDomainOrIP"`
	Taxonomy int32 `json:"taxonomy"`
	Score int32 `json:"score"`
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

var coreApiServer string = "http://"+os.Getenv("SINKIT_CORE_SERVER")+":"+os.Getenv("SINKIT_CORE_SERVER_PORT")+"/sinkit/rest/blacklist/record/"
func sinkitBackendCall(query string) (bool) {

	url := coreApiServer+query
	fmt.Println("URL:>", url)

	//var jsonStr = []byte(`{"title":"Buy cheese and bread for breakfast."}`)
	//req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonStr))
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("X-sinkit-token", os.Getenv("SINKIT_ACCESS_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
	if(len(body) < 10) {
		return false
	}

	var blacklistedRecord BlackListedRecord
	err = json.Unmarshal(body, &blacklistedRecord)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nblacklistedRecord.source[%s]\n", blacklistedRecord.Source)

	return true
}


// Dummy playground
func sinkByHostname(qname string) (bool) {
	return sinkitBackendCall(strings.TrimSuffix(qname, "."))
/*
	dummyTestHostnames := []string{"google.com", "root.cz", "seznam.cz", "youtube.com"}
	//OMG...
	sort.Strings(dummyTestHostnames)
	if (sort.SearchStrings(dummyTestHostnames, qname) == len(dummyTestHostnames)) {
		return false
	}
	return true
*/
}

// Dummy playground
func sinkByIPAddress(msg *dns.Msg) (bool) {
	/*if !aRecord.Equal(ip) {
			t.Fatalf("IP %q does not match registered IP %q", aRecord, ip)
		}*/
	//dummyTestIPAddresses := []string{"81.19.0.120"}
	//sort.Strings(dummyTestIPAddresses)
	for _, element := range msg.Answer {
		Info("\nKARMTAG: RR Element: %s\n", element)
		//if (sort.SearchStrings(dummyTestIPAddresses, element.String()) !=  len(dummyTestIPAddresses)) {
	//		return true
	//	}
		var respElemSegments = strings.Split(element.String(), "	")
		if(sinkitBackendCall((respElemSegments[len(respElemSegments)-1:])[0])) {
			return true
		}
	}
	return false
}

// Dummy playground
func sendToSinkhole(msg *dns.Msg, qname string) {
	//TODO: Isn't it a clumsy concatenation?
	var buffer bytes.Buffer
	buffer.WriteString(qname)
	buffer.WriteString("	")
	buffer.WriteString("5	")
	buffer.WriteString("IN	")
	buffer.WriteString("A	")
	buffer.WriteString("127.0.0.1")
	//Sink only the first record
	//msg.Answer[0], _ = dns.NewRR(buffer.String())
	//Sink all records:
	sinkRecord, _ := dns.NewRR(buffer.String())
	msg.Answer = []dns.RR{sinkRecord}

	Info("\n KARMTAG: A record: %s", msg.Answer[0].(*dns.A).A)
	return
}

// Lookup will ask each nameserver in top-to-bottom fashion, starting a new request
// in every second, and return as early as possible (have an answer).
// It returns an error if no request has succeeded.
func (r *Resolver) Lookup(net string, req *dns.Msg) (message *dns.Msg, err error) {
	c := &dns.Client{
		Net:          net,
		ReadTimeout:  r.Timeout(),
		WriteTimeout: r.Timeout(),
	}

	qname := req.Question[0].Name

	res := make(chan *dns.Msg, 1)
	var wg sync.WaitGroup
	L := func(nameserver string) {
		defer wg.Done()
		r, rtt, err := c.Exchange(req, nameserver)
		if err != nil {
			Debug("%s socket error on %s", qname, nameserver)
			Debug("error:%s", err.Error())
			return
		}
		if r != nil && r.Rcode != dns.RcodeSuccess {
			Debug("%s failed to get an valid answer on %s", qname, nameserver)
			return
		}
		Info("\n KARMTAG: %s resolv on %s (%s) ttl: %d\n", UnFqdn(qname), nameserver, net, rtt)
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
			Info("\n KARMTAG: Resolved to: %s\n", r.Answer)
			if (sinkByIPAddress(r) || sinkByHostname(qname)) {
				Info("\n KARMTAG: %s GOES TO SINKHOLE! XXX\n", r.Answer)
				sendToSinkhole(r, qname)
			}
			return r, nil
		case <-ticker.C:
			continue
		}
	}
	// wait for all the namservers to finish
	wg.Wait()
	select {
	case r := <-res:
	// TODO: Remove the following block, it is covered in the aforementioned loop
		Info("\n Resolved to: %s", r.Answer)
		if (sinkByIPAddress(r) || sinkByHostname(qname)) {
			Info("\n KARMTAG: %s GOES TO SINKHOLE! QQQQ\n", r.Answer)
			sendToSinkhole(r, qname)
		}
		return r, nil
	default:
		return nil, ResolvError{qname, net, r.Nameservers()}
	}

}

// Namservers return the array of nameservers, with port number appended.
// '#' in the name is treated as port separator, as with dnsmasq.
func (r *Resolver) Nameservers() (ns []string) {
	for _, server := range r.config.Servers {
		if i := strings.IndexByte(server, '#'); i > 0 {
			server = server[:i] + ":" + server[i+1:]
		} else {
			server = server + ":" + r.config.Port
		}
		ns = append(ns, server)
	}
	return
}

func (r *Resolver) Timeout() time.Duration {
	return time.Duration(r.config.Timeout) * time.Second
}
