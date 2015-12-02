package main

import (
	"testing"
	"github.com/miekg/dns"
	. "github.com/smartystreets/goconvey/convey"
	"strings"
	"log"
)

/*
	WIP, nothing interesting here.
	This is just a stub for a test playground.
*/
func TestBlacklistedDomains(t *testing.T) {
	letters := []string{"seznam.cz", "youtube.com", "root.cz", "google.com", "karms.biz"}

	Convey("Bla bla bla", t, func() {
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(letters[0]), dns.TypeMX)
		m.Id = dns.Id()
		m.RecursionDesired = true
		c := new(dns.Client)

		r, _, err := c.Exchange(m, "127.0.0.1:53535")
		Convey("Sinked", func() {
			So(err, ShouldBeNil)
			// IP address of the sinkhole....
			So(r.Answer, ShouldContain, "127.0.0.1")
		})
	})
}

func TestAnswerParsing(t *testing.T) {
	letters := []string{"www.cnn.com", "www.gmail.com", "www.newsforge.com", "L.GOOGLE.COM", "dns-admin.GOOGLE.COM"}

	Convey("Parsing", t, func() {
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(letters[0]), dns.TypeA)
		m.Id = dns.Id()
		m.RecursionDesired = true
		c := new(dns.Client)

		r, _, err := c.Exchange(m, "8.8.8.8:53")
		Convey("Result", func() {
			So(err, ShouldBeNil)
			for _, element := range r.Answer {
				log.Printf("\nKARMTAG: RR Element: %s\n", element)
				vals := strings.Split(element.String(), "	")
				for i := range vals {
					log.Printf("KARMTAG: value: %s\n", vals[i])
					if(strings.EqualFold(vals[i], "A") || strings.EqualFold(vals[i], "CNAME") || strings.EqualFold(vals[i], "AAAA")) {
						log.Printf("KARMTAG: value matches: %s\n", vals[i])
						log.Printf("KARMTAG: len(vals) is %s, i+1 is %s\n", len(vals), i+1)
						if(len(vals)>=i+1) {
							log.Printf("KARMTAG: to send to Core: %s\n", vals[i+1])
							break
						}
					} else {
						log.Printf("KARMTAG: value doesn't match: %s\n", vals[i])
					}
				}
			}
		})
	})
}
