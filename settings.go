package main

import (
	"github.com/kelseyhightower/envconfig"
	"log"
	"runtime"
	"crypto/x509"
	"crypto/tls"
	"encoding/base64"
)

var (
	settings Settings
	credentials Credentials
)

var LogLevelMap = map[string]int{
	"DEBUG":    LevelDebug,
	"INFO":     LevelInfo,
	"NOTICE":   LevelNotice,
	"WARN":     LevelWarn,
	"ERROR":    LevelError,
}

var AuditLevelMap = map[string]int{
	"ALL":     AllEvents,
	"AUDITED": AuditedEvents,
	"BLOCKED": BlockedEvents,
}

type Credentials struct {
	clientKeyPair tls.Certificate
	caCertPool    *x509.CertPool
}

type Settings struct {
	BIND_HOST                       string
	BIND_PORT                       int
	GODNS_READ_TIMEOUT              int
	GODNS_WRITE_TIMEOUT             int
	GODNS_UDP_PACKET_SIZE           int
	RESOLV_CONF_FILE                string
	BACKEND_RESOLVER_RW_TIMEOUT     int
	BACKEND_RESOLVER_TICK           int
	LOG_STDOUT                      bool
	LOG_FILE                        string
	AUDIT_FILE                      string
	LOG_LEVEL                       string
	AUDIT_LEVEL                     string
	ORACULUM_CACHE_BACKEND          string
	ORACULUM_CACHE_EXPIRE           int
	ORACULUM_CACHE_MAXCOUNT         int
	LOCAL_RESOLVER                  bool
	INSECURE_SKIP_VERIFY            bool
	CLIENT_CRT_BASE64               string
	CLIENT_KEY_BASE64               string
	CA_CRT_BASE64                   string
	CLIENT_ID                       int
	CLIENT_ID_HEADER                string
	BACKEND_RESOLVERS               []string
	BACKEND_RESOLVERS_EXCLUSIVELY   bool
	BACKEND_RESOLVERS_TCP_ONLY      bool
	ORACULUM_API_FIT_TIMEOUT        int64
	ORACULUM_SLEEP_WHEN_DISABLED    int64
	ORACULUM_API_TIMEOUT            int64
	SINKHOLE_ADDRESS                string
	SINKHOLE_TTL                    int
	ORACULUM_ACCESS_TOKEN_VALUE     string
	ORACULUM_ACCESS_TOKEN_KEY       string
	ORACULUM_DISABLED               bool
	ORACULUM_URL                    string
	ORACULUM_IP_ADDRESSES_ENABLED   bool
	ORACULUM_API_MAX_REQUESTS       int
	Version                         string
	NUM_OF_CPUS                     int
	CACHE_URL                       string
	CACHE_REFRESH_IOC               int
	CACHE_REFRESH_CUSTOMLIST        int
	CACHE_REFRESH_IOCWITHCUSTOMLIST int
	CACHE_RETRY_COUNT               int
	CACHE_RETRY_INTERVAL            int
	CACHE_REQUEST_TIMEOUT           int
}

func (s Settings) LogLevel() int {
	l, ok := LogLevelMap[s.LOG_LEVEL]
	if !ok {
		panic("Config error: invalid log level: " + s.LOG_LEVEL)
	}
	return l
}

func (s Settings) AuditLevel() int {
	if s.AUDIT_LEVEL != "" {
		l, ok := AuditLevelMap[s.AUDIT_LEVEL]
		if !ok {
			panic("Config error: invalid audit level: " + s.AUDIT_LEVEL)
		}
		return l
	} else {
		return AllEvents
	}
}

func init() {
	err := envconfig.Process("SINKIT", &settings)
	if err != nil {
		log.Fatal(err.Error())
	}
	// TODO: This is garbage, needs to pull version from a meta file, git release hook etc.
	settings.Version = "0.5.0"
	if (settings.NUM_OF_CPUS == 0 || settings.NUM_OF_CPUS > runtime.NumCPU()) {
		settings.NUM_OF_CPUS = runtime.NumCPU()
	}
	log.Println("Settings loaded.")

	// TODO: validation and sensible defaults for settings env props...

	if (settings.LOCAL_RESOLVER) {
		//1369 magic number: base64 encoded 1024 ASCII chars.
		if (len(settings.CA_CRT_BASE64) < 1369) {
			log.Fatalf("SINKIT_CA_CRT_BASE64 env var string is too short to be valid, content: %s", settings.CA_CRT_BASE64)
		}
		if (len(settings.CLIENT_CRT_BASE64) < 1369) {
			log.Fatalf("SINKIT_CLIENT_CRT_BASE64 env var string is too short to be valid, content: %s", settings.CLIENT_CRT_BASE64)
		}
		if (len(settings.CLIENT_KEY_BASE64) < 1369) {
			log.Fatalf("SINKIT_CLIENT_KEY_BASE64 env var string is too short to be valid, content: %s", settings.CLIENT_KEY_BASE64)
		}
		if (settings.CLIENT_ID < 1) {
			log.Println("SINKIT_CLIENT_ID env var is not set to a meaningful int value. It will not be used.")
		} else {
			if (len(settings.CLIENT_ID_HEADER) < 1) {
				log.Println("SINKIT_CLIENT_ID env var is set, but the SINKIT_CLIENT_ID_HEADER seems too short to be valid.")
			}
		}
		if (settings.INSECURE_SKIP_VERIFY) {
			log.Println("SINKIT_INSECURE_SKIP_VERIFY is set to true. This is valid only in local testing environment.")
		}
		if settings.ORACULUM_API_MAX_REQUESTS < 1 {
			log.Fatalf("SINKIT_ORACULUM_API_MAX_REQUESTS env var is too low to be valid, content: %s", settings.ORACULUM_API_MAX_REQUESTS)
		}
		//log.Println(settings.CLIENT_CRT_BASE64)
		//log.Println(settings.CLIENT_KEY_BASE64)
		//log.Println(settings.CA_CRT_BASE64)

		clientCert, err := base64.StdEncoding.DecodeString(settings.CLIENT_CRT_BASE64)
		//clientCert, err := base64.StdEncoding.DecodeString(clientCertPEMBase64)

		//log.Println("@" + settings.CLIENT_CRT_BASE64 + "@")
		//log.Println("@" + os.Getenv("SINKIT_CLIENT_CRT_BASE64") + "@")
		//log.Println("@" + clientCertPEMBase64 + "@")

		if err != nil {
			log.Fatal(err.Error())
		}
		clientKey, err := base64.StdEncoding.DecodeString(settings.CLIENT_KEY_BASE64)
		//clientKey, err := base64.StdEncoding.DecodeString(clientKeyPEMBase64)
		if err != nil {
			log.Fatal(err.Error())
		}
		caCert, err := base64.StdEncoding.DecodeString(settings.CA_CRT_BASE64)
		//caCert, err := base64.StdEncoding.DecodeString(caCertPEMBase64)
		if err != nil {
			log.Fatal(err.Error())
		}
		keyPair, err := tls.X509KeyPair(clientCert, clientKey)
		if err != nil {
			log.Fatal(err.Error())
		}
		credentials.clientKeyPair = keyPair
		credentials.caCertPool = x509.NewCertPool()
		credentials.caCertPool.AppendCertsFromPEM(caCert)
		log.Println("Credentials loaded.")
	}

	initCoreClient(settings)
}
