// Package hath is Hentai@Home client implementation in golang
package hath // import "cydev.ru/hath"

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo"
)

// Server should handle requests from users (and rpc?)
// speedtests
// server commands:
//     /servercmd/<command>/<additional:kwds>/<timestamp:int>/<key>'
// image file request:
//     /h/<fileid>/<additional:kwds>/<filename>
type Server interface {
	http.Handler
}

// DefaultServer uses hard drive to respond
type DefaultServer struct {
	cfg      ServerConfig
	frontend Frontend
	db       DataBase
	e        *echo.Echo
	useQuery chan File
	wg       *sync.WaitGroup
	started  bool
}

const (
	argsDelimiter     = ";"
	argsEqual         = "="
	argsKeystamp      = "keystamp"
	timestampMaxDelta = 60
	useQuerySize      = 100
)

// ProxyMode sets proxy security politics
type ProxyMode byte

const (
	// ProxyDisabled no requests allowed
	ProxyDisabled ProxyMode = iota + 1 // starts with 1
	// ProxyLocalNetworksProtected allows requests from local network with passkey
	ProxyLocalNetworksProtected // 2
	// ProxyLocalNetworksOpen allows any requests from local network
	ProxyLocalNetworksOpen // 3
	// ProxyAllNetworksProtected allows requests from any network with passkey (not recommended)
	ProxyAllNetworksProtected // 4
	// ProxyAllNetworksOpen allows any requests from any network (very not recommended)
	ProxyAllNetworksOpen // 5
)

const (
	proxyModeMin  = int64(ProxyDisabled)
	proxyModeMax  = int64(ProxyAllNetworksOpen)
	proxyModeBits = 4
)

func (p ProxyMode) String() string {
	if p == ProxyDisabled {
		return "disabled"
	}
	if p == ProxyLocalNetworksOpen {
		return "open for local networks"
	}
	if p == ProxyAllNetworksProtected {
		return "local networks with passkey"
	}
	if p == ProxyAllNetworksProtected {
		return "all networks with passkey"
	}
	if p == ProxyAllNetworksOpen {
		return "open for all networks"
	}
	return "unknown"
}

// Args represents additional arguments in request string
type Args map[string]string

// Get returns string value
func (a Args) Get(k string) string {
	return a[k]
}

// GetInt parses and returns integer value
func (a Args) GetInt(k string) int {
	i, _ := strconv.Atoi(a[k])
	return i
}

// GetInt64 parses and returns 64bit integer value
func (a Args) GetInt64(k string) int64 {
	i, _ := strconv.ParseInt(a[k], 10, 64)
	return i
}

func (a Args) String() string {
	var elems []string
	for k, v := range a {
		elems = append(elems, strings.Join([]string{k, v}, argsEqual))
	}
	return strings.Join(elems, argsDelimiter)
}

// ParseArgs parses arguments from string
//     arg1=val1;arg2=val2
func ParseArgs(s string) (a Args) {
	a = make(Args)
	components := strings.Split(s, argsDelimiter)
	for _, component := range components {
		kw := strings.Split(component, argsEqual)
		if len(kw) != 2 {
			continue
		}
		a[kw[0]] = kw[1]
	}
	return a
}

// handleImage /h/<fileid>/<additional:kwds>/<filename>
func (s *DefaultServer) handleImage(c *echo.Context) error {
	fileID := c.Param("fileid")
	args := ParseArgs(c.Param("kwds"))
	// parsing timestamp and keystamp
	stamps := strings.Split(args.Get(argsKeystamp), keyStampDelimiter)
	if len(stamps) != 2 {
		return c.HTML(http.StatusBadRequest, "400: Bad stamp format")
	}
	currentTimestamp := time.Now().Unix()
	timestamp, err := strconv.ParseInt(stamps[0], 10, 64)
	if err != nil {
		return c.HTML(http.StatusBadRequest, "400: Bad timestamp")
	}
	deltaTimestamp := currentTimestamp - timestamp
	if deltaTimestamp < 0 {
		deltaTimestamp *= -1
	}
	if deltaTimestamp > timestampMaxDelta {
		return c.HTML(http.StatusBadRequest, "400: timestamp delta is too big")
	}
	keyStamp := stamps[1]
	f, err := FileFromID(fileID)
	if err != nil {
		return c.HTML(http.StatusBadRequest, "400: bad file id")
	}
	expectedKeyStamp := f.KeyStamp(s.cfg.Key, timestamp)
	if expectedKeyStamp != keyStamp {
		return c.HTML(http.StatusForbidden, "403: bad keystamp")
	}
	s.useQuery <- f
	return s.frontend.Handle(f, c.Response().Writer())
}

func (s *DefaultServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.e.ServeHTTP(w, r)
}

// lastUsage update loop
func (s *DefaultServer) useLoop() {
	defer s.wg.Done()
	for f := range s.useQuery {
		if err := s.db.Use(f); err != nil {
			log.Printf("db miss for %s, writing to database\n", f.HexID())
			if err := s.db.Add(f); err != nil {
				log.Println("db error while adding file", f)
			}
		}
	}
}

// Start server internal goroutines
func (s *DefaultServer) Start() error {
	// starting lastUsage update loop
	s.wg.Add(1)
	go s.useLoop()

	s.started = true
	return nil
}

// Close stops server
func (s *DefaultServer) Close() error {
	close(s.useQuery)
	s.db.Close()
	s.wg.Wait()

	s.started = false
	return nil
}

// ServerConfig cfg for server
type ServerConfig struct {
	Credentials
	Frontend Frontend
	DataBase DataBase
}

// NewServer cleares default server with provided client and frontend
func NewServer(cfg ServerConfig) *DefaultServer {
	s := new(DefaultServer)
	s.cfg = cfg
	s.db = cfg.DataBase
	s.frontend = cfg.Frontend
	e := echo.New()
	e.Get("/h/:fileid/:kwds/:filename", s.handleImage)
	s.e = e
	s.useQuery = make(chan File, useQuerySize)
	s.wg = new(sync.WaitGroup)
	return s
}
