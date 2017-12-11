package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"regexp"
)

type Sinkhole struct {
	Sinkhole string `json:"sinkhole"`
}

type CoreError struct {
	When time.Time
	What string
	URL  string
}

func (e CoreError) Error() string {
	return fmt.Sprintf("%v: %v for %v", e.When, e.What, e.URL)
}

type apiCallItem struct {
	request *http.Request
}

var (
	coreDisabled uint32 = 0
	disabledSecondsTimestamp int64 = 0
	apiCallBucket chan *apiCallItem
	validQueryOrAddress, _ = regexp.Compile("^[a-zA-Z-_\\.0-9:]+$")
)

func init() {
}

func dryAPICall(query string, clientAddress string, trimmedQname string) {
	if atomic.LoadInt64(&disabledSecondsTimestamp) == 0 {
		logger.Debug("disabledSecondsTimestamp was 0, setting it to the current time")
		atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
		return
	}
	currentTime := int64(time.Now().Unix())
	lastStamp := atomic.LoadInt64(&disabledSecondsTimestamp)
	if (currentTime - lastStamp) * 1000 > settings.ORACULUM_SLEEP_WHEN_DISABLED {
		logger.Debug("Doing dry API call...")
		start := time.Now()
		//Doesn't hurt IP
		doAPICall(trimmedQname, clientAddress, trimmedQname)
		elapsed := time.Since(start)
		/* Error code is not a factor in Disabled decision any more
		if err != nil {
			logger.Error("Core remains DISABLED. Gonna wait. Error: %s", err)
			atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
			return
		}*/
		if elapsed > time.Duration(settings.ORACULUM_API_FIT_TIMEOUT) * time.Millisecond {
			logger.Error("Core remains DISABLED. Gonna wait. Elapsed time: %s, FitResponseTime: %s", elapsed, time.Duration(settings.ORACULUM_API_FIT_TIMEOUT) * time.Millisecond)
			atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
			return
		}
		logger.Error("Core is now ENABLED")
		atomic.StoreUint32(&coreDisabled, 0)
	} else {
		logger.Debug("Not enough time passed, waiting for another call. Elapsed: %s ms, Limit: %s ms", (currentTime - lastStamp) * 1000, settings.ORACULUM_SLEEP_WHEN_DISABLED)
	}
	return
}

func initBuckets() {
	// prefill api call bucket channel
	apiCallBucket = make(chan *apiCallItem, settings.ORACULUM_API_MAX_REQUESTS)
	for i := 0; i < cap(apiCallBucket); i++ {
		req, err := http.NewRequest("GET", "", nil)
		if err != nil {
			logger.Error("Could not initialize http Request for core: ", err)
		}
		req.Header.Set(settings.ORACULUM_ACCESS_TOKEN_KEY, settings.ORACULUM_ACCESS_TOKEN_VALUE)
		req.Header.Set("Content-Type", "application/json")
		if settings.CLIENT_ID > 0 {
			req.Header.Set(settings.CLIENT_ID_HEADER, strconv.Itoa(settings.CLIENT_ID))
		}
		apiCallBucket <- &apiCallItem{
			request: req,
		}
	}
}

func dryAPICallBucket(trimmedQname string, clientAddress string) error {
	if apiCallBucket == nil {
		initBuckets()
	}

	select {
	case item := <-apiCallBucket:
		apiURL := createAPIUrl(clientAddress, trimmedQname, trimmedQname)
		logger.Debug("URL:>", apiURL)
		item.request.URL, _ = url.Parse(apiURL)
		_, err := runAPICall(item.request)
		apiCallBucket <- item
		return err
	default:
	// rate exceeded, ignore call
		logger.Warn("API request limit reached, skipping " + trimmedQname)
		return nil // BucketError{time.Now(), "Dry API call rate exceeded"}
	}
}

func doAPICall(query string, clientAddress string, trimmedQname string) (value bool, err error) {
	url := createAPIUrl(clientAddress, query, trimmedQname)
	logger.Debug("URL:>", url)

	//var jsonStr = []byte(`{"Key":"Something Else"}`)
	//req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonStr))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Error("Could not initialize http Request for core: ", err)
		return false, err
	}
	req.Header.Set(settings.ORACULUM_ACCESS_TOKEN_KEY, settings.ORACULUM_ACCESS_TOKEN_VALUE)
	req.Header.Set("Content-Type", "application/json")
	if settings.CLIENT_ID > 0 {
		req.Header.Set(settings.CLIENT_ID_HEADER, strconv.Itoa(settings.CLIENT_ID))
	}

	return runAPICall(req)
}

func runAPICall(req *http.Request) (value bool, err error) {
	resp, err := CoreClient.Do(req)
	if err != nil {
		logger.Debug("There has been an error with backend.")
		return false, err
	}
	defer resp.Body.Close()

	logger.Debug("Response Status:", resp.Status)
	logger.Debug("Response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		logger.Debug("Response Body:", string(body))
		return false, CoreError{time.Now(), "Non HTTP 200.", req.URL.String()}
	}
	// i.e. "null" or possible stray byte, not a sinkhole IP
	if len(body) < 6 {
		logger.Debug("Response short.")
		return false, nil
	}

	var sinkhole Sinkhole
	//TODO Use Sinkole instead of env property
	err = json.Unmarshal(body, &sinkhole)
	if err != nil {
		logger.Debug("There has been an error with unmarshalling the response: %s", body)
		return false, err
	}
	logger.Debug("\nSINKHOLE RETURNED from Core[%s]\n", sinkhole.Sinkhole)

	return true, nil
}

func sinkitBackendCall(query string, clientAddress string, trimmedQname string, oraculumCache Cache, cacheOnly bool) bool {
	key := RequestHash(query, trimmedQname, clientAddress)

	answer, err := oraculumCache.Get(key)
	if err == nil {
		return answer
	}

	if cacheOnly {
		return false
	}

	start := time.Now()
	goToSinkhole, err := doAPICall(query, clientAddress, trimmedQname)
	elapsed := time.Since(start)
	/* Error code is not a factor in Disabled decision any more
	if err != nil {
		atomic.StoreUint32(&coreDisabled, 1)
		atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
		logger.Error("Core was DISABLED. Error: %s, source: %s", err, clientAddress)
		return false
	}*/
	if elapsed > time.Duration(settings.ORACULUM_API_FIT_TIMEOUT) * time.Millisecond {
		atomic.StoreUint32(&coreDisabled, 1)
		atomic.StoreInt64(&disabledSecondsTimestamp, int64(time.Now().Unix()))
		logger.Error("Core was DISABLED. Elapsed time: %s, FitResponseTime: %s, Query: %s, source: %s", elapsed, time.Duration(settings.ORACULUM_API_FIT_TIMEOUT) * time.Millisecond, trimmedQname, clientAddress)
		return false
	}

	oraculumCache.Set(key, goToSinkhole)

	return goToSinkhole
}

func sinkByHostname(trimmedQname string, clientAddress string, oraculumCache Cache, cacheOnly bool) bool {
	// Yes, twice trimmedQname
	return sinkitBackendCall(trimmedQname, clientAddress, trimmedQname, oraculumCache, cacheOnly)
}

// We do not sinkhole here, the side effect is that CNAMEs slip through.
func sinkByIPAddress(msg *dns.Msg, clientAddress string, trimmedQname string, oraculumCache Cache, cacheOnly bool) {
	for _, element := range msg.Answer {
		logger.Debug("\nKARMTAG: RR Element: %s\n", element)
		vals := strings.Split(element.String(), "	")
		// We loop through the elements, TTL, IN, Class...
		for i := range vals {
			logger.Debug("KARMTAG: value: %s\n", vals[i])
			if strings.EqualFold(vals[i], "A") || strings.EqualFold(vals[i], "CNAME") || strings.EqualFold(vals[i], "AAAA") {
				logger.Debug("KARMTAG: value matches: %s\n", vals[i])
				// Length in bytes, not runes. Shorter doesn't make sense.
				if len(vals) > i + 1 && isAnswerValid(vals[i + 1]) {
					logger.Debug("KARMTAG: to send to Core: %s\n", vals[i + 1])
					go sinkitBackendCall(strings.TrimSuffix(vals[i + 1], "."), clientAddress, trimmedQname, oraculumCache, cacheOnly)
				}
				break
			}
		}
	}
}

func processCoreCom(msg *dns.Msg, qname string, clientAddress string, oraculumCache Cache, caches *ListCache) {
	// Don't bother contacting Infinispan Sinkit Core
	if settings.ORACULUM_DISABLED {
		logger.Debug("SINKIT_RESOLVER_DISABLE_INFINISPAN TRUE\n")
		return
	}
	logger.Debug("SINKIT_RESOLVER_DISABLE_INFINISPAN FALSE or N/A\n")
	logger.Debug("\n KARMTAG: Resolved to: %s\n", msg.Answer)

	trimmedQname := strings.TrimSuffix(qname, ".")

	if !isDNSRequestValid(trimmedQname, clientAddress) {
		return
	}

	qnamesMD5 := genSubdomainsMD5(trimmedQname)
	var (
		err    error
		action tAction
	)
	if settings.LOCAL_RESOLVER {
		// check customlist - log/block/white
		action, err = caches.Customlist.FindFirstKey(qnamesMD5)
		if err == nil {
			if action == ActionWhite {
				logger.Debug("\n KARMTAG: Record %s is allowed in customlist", qname)
			} else {
				// block or log only
				if action == ActionBlack {
					logger.Debug("Record %s is blocked in customlist.\n", qname)
					auditor.Blocked(clientAddress, trimmedQname)
					sendToSinkhole(msg, qname)
				} else {
					logger.Debug("\n KARMTAG: Record %s is audited by customlist", qname)
					auditor.Audited(clientAddress, trimmedQname)
				}
				go dryAPICallBucket(trimmedQname, clientAddress)
			}
			return
		}

		// check ioclist, only log/block
		action, err = caches.Ioclist.FindFirstKey(qnamesMD5)
		if err == nil {
			if action == ActionLog {
				logger.Debug("\n KARMTAG: Record %s is audited by ioclist", qname)
				auditor.Audited(clientAddress, trimmedQname)
			} else {
				logger.Debug("Record %s is blocked in ioclist.\n", qname)
				auditor.Blocked(clientAddress, trimmedQname)
				sendToSinkhole(msg, qname)
			}
			go dryAPICallBucket(trimmedQname, clientAddress)
		}
		// for LR end here
		return
	}

	// check list with IoCs and Custom lists, only records found here are checked
	action, err = caches.AllIoCwithCustomLists.FindFirstKey(qnamesMD5)
	if err == nil {
		if action == ActionCheck {
			coreDisabledNow := atomic.LoadUint32(&coreDisabled) == 1
			if coreDisabledNow {
				logger.Debug("Core is DISABLED. Gonna call dryAPICall.")
				go dryAPICall(trimmedQname, clientAddress, trimmedQname)
			}
			if settings.ORACULUM_IP_ADDRESSES_ENABLED {
				sinkByIPAddress(msg, clientAddress, trimmedQname, oraculumCache, coreDisabledNow)
				// We do not sinkhole based on IP address here.
			}
			if sinkByHostname(trimmedQname, clientAddress, oraculumCache, coreDisabledNow) {
				logger.Debug("Record %s is blocked by core API call.\n", msg.Answer)
				auditor.Blocked(clientAddress, trimmedQname)
				sendToSinkhole(msg, qname)
			}
		} // no other Action expected to trigger Core API call at this time...
	}
}

func sendToSinkhole(msg *dns.Msg, qname string) {
	var buffer bytes.Buffer
	buffer.WriteString(qname)
	buffer.WriteString("	")
	buffer.WriteString(strconv.Itoa(settings.SINKHOLE_TTL))
	buffer.WriteString("	")
	buffer.WriteString("IN	")
	buffer.WriteString("A	")
	buffer.WriteString(settings.SINKHOLE_ADDRESS)
	sinkRecord, _ := dns.NewRR(buffer.String())
	msg.Answer = []dns.RR{sinkRecord}
	return
}

func createAPIUrl(clientAddress string, query string, trimmedQname string) string {
	var bufferQuery bytes.Buffer
	bufferQuery.WriteString(settings.ORACULUM_URL)
	bufferQuery.WriteString("/")
	bufferQuery.WriteString(clientAddress)
	bufferQuery.WriteString("/")
	bufferQuery.WriteString(query)
	bufferQuery.WriteString("/")
	bufferQuery.WriteString(trimmedQname)
	return bufferQuery.String()
}

// RequestHash computes hash of dns query
func RequestHash(query string, trimmedQname string, clientAddress string) string {
	keygen := md5.New()
	var buffer bytes.Buffer
	buffer.WriteString(query)
	if !settings.LOCAL_RESOLVER {
		buffer.WriteString(clientAddress)
	}
	buffer.WriteString(trimmedQname)
	keygen.Write(buffer.Bytes())
	return hex.EncodeToString(keygen.Sum(nil))
}

func qnameToMD5(trimmedQname string) string {
	var buffer bytes.Buffer
	keygen := md5.New()
	buffer.WriteString(trimmedQname)
	keygen.Write(buffer.Bytes())
	return hex.EncodeToString(keygen.Sum(nil))
}

func isDNSRequestValid(trimmedQname string, clientAddress string) bool {
	if !validQueryOrAddress.MatchString(trimmedQname) {
		logger.Warn("trimmedQname `%s' contained an illegal character.\n", trimmedQname)
		return false
	}
	if len(clientAddress) < 3 || len(clientAddress) > 41 {
		logger.Warn("Client address is too short or too long %s\n", clientAddress)
		return false
	}
	if len(trimmedQname) < 3 || len(trimmedQname) > 250 {
		logger.Warn("Query FQDN is likely invalid: %s\n", trimmedQname)
		return false
	}

	return true
}

func isAnswerValid(answer string) bool {
	if strings.HasSuffix(answer, ".root-servers.net.") {
		return false
	}
	if !validQueryOrAddress.MatchString(answer) {
		logger.Warn("answer `%s' contained an illegal character.\n", answer)
		return false
	}
	if len(answer) < 3 || len(answer) > 250 {
		logger.Warn("Answer is likely invalid: %s\n", answer)
		return false
	}

	return true
}

// genSubdomainsMD5 generates array of md5 hashes for all subdomains, longest first, stops at 2nd level domain: a.b.c.d -> ["a.b.c.d", "b.c.d", "c.d"]
func genSubdomainsMD5(trimmedQname string) []string {
	parts := strings.Split(trimmedQname, ".")
	res := make([]string, len(parts)-1)
	for i := range res[:len(parts)-1] {
		res[i] = qnameToMD5(strings.Join(parts[i:], "."))
	}
	return res
}
