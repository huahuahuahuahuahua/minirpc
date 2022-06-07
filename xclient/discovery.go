package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

//SelectMode 代表不同的负载均衡策略，简单起见，miniRPC 仅实现 Random 和 RoundRobin 两种策略。
type SelectMode int

const (
	RandomSelect     SelectMode = iota // select randomly
	RoundRobinSelect                   // select using Robbin algorithm  轮询模式
)

//Discovery 是一个接口类型，包含了服务发现所需要的最基本的接口。
type Discovery interface {
	Refresh() error // refresh from remote registry
	Update(servers []string )error
	Get(mode SelectMode)(string,error)
	GetAll()([]string,error)
}

//实现一个不需要注册中心，服务列表由手工维护的服务发现的结构体：MultiServersDiscovery

// MultiServersDiscovery is a discovery for multi servers without a registry center
// user provides the server addresses explicitly instead

type MultiServersDiscovery struct {
	r *rand.Rand  // generate random number
	mu sync.RWMutex // protect following
	servers []string
	index int  // record the selected position for robin algorithm
}


func NewMultiServerDiscovery(servers []string) *MultiServersDiscovery  {
	d := &MultiServersDiscovery{
		servers: servers,
		//随机数生成器，加入时间戳保证每次生成的随机数不一样
		r:rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	//interval [0,n). It panics if n <= 0.
	d.index = d.r.Intn(math.MaxInt32-1)
	return d
}

var _ Discovery = (*MultiServersDiscovery)(nil)

func (d MultiServersDiscovery) Refresh() error {
	return nil
}


func (d *MultiServersDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	return nil
}

func (d *MultiServersDiscovery) Get(mode SelectMode) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}
	switch mode {
	case RandomSelect:
		return d.servers[d.r.Intn(n)],nil
	case RoundRobinSelect:
		s:=d.servers[d.index%n] // servers could be updated, so mode n to ensure safety
		d.index = (d.index+1)%n
		return s,nil
	default:
		return "", errors.New("rpc discovery: not supported select mode")
	}
}

func (d *MultiServersDiscovery) GetAll() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	servers:=make([]string,len(d.servers),len(d.servers))
	copy(servers,d.servers)
	return servers,nil
}