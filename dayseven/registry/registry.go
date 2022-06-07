package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// GeeRegistry is a simple register center, provide following functions.
// add a server and receive heartbeat to keep it alive.
// returns all alive servers and delete dead servers sync simultaneously.

type ServerItem struct {
	Addr string
	start time.Time
}
//定义 GeeRegistry 结构体，默认超时时间设置为 5 min，
//任何注册的服务超过 5 min，即视为不可用状态。

type MiniRegistry struct {
	timeout time.Duration
	mu sync.Mutex
	servers map[string]*ServerItem
}

const (
	defaultPath ="/_minirpc_/registry"
	defaultTimeout = time.Minute * 5
)

func New(timeout time.Duration) *MiniRegistry  {
	return &MiniRegistry{
		servers:make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultGeeRegister = New(defaultTimeout)

func (r *MiniRegistry) putServer(addr string)  {
	r.mu.Lock()
	defer r.mu.Unlock()
	s:=r.servers[addr]
	if s==nil {
		r.servers[addr] =&ServerItem{Addr: addr,start:time.Now()}
	}else{
		s.start = time.Now() // if exists, update start time to keep alive
	}
}

func (r MiniRegistry) aliveServers() []string  {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr,s := range r.servers{
		if r.timeout==0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive,addr)
		}else {
			delete(r.servers,addr)
		}
	}
	sort.Strings(alive)
	return alive
}

// Runs at /_geerpc_/registry
func (r *MiniRegistry)ServeHTTP(w http.ResponseWriter,req *http.Request)  {
	switch req.Method {
	case "GET":
		w.Header().Set("X-Minirpc-Servers",strings.Join(r.aliveServers(),","))
	case "POST":
		addr:=req.Header.Get("X-Minirpc-Server")
		if addr=="" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// HandleHTTP registers an HTTP handler for GeeRegistry messages on registryPath
func (r *MiniRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultGeeRegister.HandleHTTP(defaultPath)
}

// Heartbeat send a heartbeat message every once in a while
// it's a helper function for a server to register or send heartbeat

func Heartbeat(registry,addr string,duration time.Duration)  {
	if duration==0 {
		duration =defaultTimeout -time.Duration(1)*time.Minute

	}
	var err error
	err = sendHeartbeat(registry, addr)
	go func() {
		t:=time.NewTicker(duration)
		for err==nil {
			<-t.C
			err = sendHeartbeat(registry,addr)
		}
	}()
}
//提供 Heartbeat 方法，便于服务启动时定时向注册中心发送心跳，默认周期比注册中心设置的过期时间少 1 min。
func sendHeartbeat(registry string, addr string) error {
	log.Println(addr,"send heart beat to registry",registry)
	httpClient:=&http.Client{}
	req,_:=http.NewRequest("POST",registry,nil)
	req.Header.Set("X-minirpc-Server",addr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}