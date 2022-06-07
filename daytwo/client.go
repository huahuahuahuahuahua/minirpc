package minirpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"minirpc/codec"
	"net"
	"sync"
)

// Client represents an RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.

//结构体 Call 来承载一次 RPC 调用所需要的信息

type Call struct {
	Seq uint64
	ServiceMethod string // format "<service>.<method>"
	Args interface{} // arguments to the function
	Reply interface{} // reply from the function
	Error error
	//Go语言中的通道（channel）是一种特殊的类型。在任何时候，
	//同时只能有一个 goroutine 访问通道进行发送和获取数据。
	//goroutine 间通过通道就可以通信。
	//var 通道变量 chan 通道类型
	Done chan *Call // Strobes when call is complete.
}

func (call *Call) done()  {
	call.Done <- call
}


type Client struct {
	cc codec.Codec 	//cc 是消息的编解码器，和服务端类似，用来序列化将要发送出去的请求，以及反序列化接收到的响应。
	opt *Option
	//sync包和channel机制来解决并发机制中不同goroutine之间的同步和通信
	//sync.Mutex是一个互斥锁，可以由不同的goroutine加锁和解锁。
	sending sync.Mutex //sending 是一个互斥锁，和服务端类似，为了保证请求的有序发送，即防止出现多个请求报文混淆。
	header codec.Header //header 是每个请求的消息头，header 只有在请求发送时才需要，而请求发送是互斥的，因此每个客户端只需要一个，声明在 Client 结构体中可以复用。
	mu sync.Mutex // protect following
	seq uint64 //seq 用于给发送的请求编号，每个请求拥有唯一编号。
	pending map[uint64]*Call //pending 存储未处理完的请求，键是编号，值是 Call 实例。
	closing bool // user has called Close
	shutdown bool // server has told us to stop
}


var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")


func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

func (client *Client)IsAvailable() bool  {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing
}
//将参数 call 添加到 client.pending 中，并更新 client.seq。
func (client *Client) registerCall(call *Call)(uint64,error)  {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0,ErrShutdown
	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq,nil
}
//根据 seq，从 client.pending 中移除对应的 call，并返回。
func (client *Client) removeCall(seq uint64) *Call  {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending,seq)
	return call
}
//：服务端或客户端发生错误时调用，
//将 shutdown 设置为 true，且将错误信息通知所有 pending 状态的 call。
func (client Client) terminateCalls(err error)  {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown =true
	for _,call :=range client.pending{
		call.Error = err
		call.done()
	}
}

func (client Client) receive()  {
	var err error
	for err==nil {
		var h codec.Header
		if err=client.cc.ReadHeader(&h);err!=nil {
			break
		}
		call:=client.removeCall(h.Seq)
		switch  {
		case call==nil:
		// it usually means that Write partially failed
		// and call was already removed.
		err = client.cc.ReadBody(nil)
		case h.Error!="":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadHeader(nil)
			call.done()
		default:
			if err=client.cc.ReadBody(call.Reply);err != nil {
				call.Error = errors.New("reading body" + err.Error())
			}
			call.done()
		}
	}
	client.terminateCalls(err)
}
//创建 Client 实例时，首先需要完成一开始的协议交换，即发送 Option 信息给服务端。
//协商好消息的编解码方式之后，再创建一个子协程调用 receive() 接收响应。

func NewClient(conn net.Conn,opt *Option)(*Client,error)  {
	f:=codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err:=fmt.Errorf("invalid codec type %s",opt.CodecType)
		log.Println("rpc client:codec error :",err)
		return nil, err
	}
	if err:=json.NewEncoder(conn).Encode(opt);err != nil {
		log.Println("rpc client:options error:",err)
		_=conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn),opt),nil
}
//协商好消息的编解码方式之后，再创建一个子协程调用 receive() 接收响应。
func newClientCodec(cc codec.Codec, opt *Option) *Client {
	client:=&Client{
		seq:1,
		cc:cc,
		opt:opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}
//为了简化用户调用，通过 ...*Option 将 Option 实现为可选参数。
func parseOptions(opts ...*Option)(*Option,error)  {
	//	if opts is nil or pass nil as parameter
	if len(opts) == 0|| opts[0] == nil{
		return DefaultOption,nil
	}
	if len(opts)!=1 {
		return nil,errors.New("number of options is more than 1")
	}
	opt :=opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType=="" {
		opt.CodecType =DefaultOption.CodecType
	}
	return opt,nil
}

// Dial connects to an RPC server at the specified network address
func  Dial(network,address string,opts ...*Option)(client *Client,err error)  {
	opt,err := parseOptions(opts...)
	if err != nil {
		return nil,err
	}
	//
	//创建连接
	//conn,err := net.Dial("tcp","192.168.1.254:4001")
	//参数说明如下：
	//network 参数表示传入的网络协议（比如 tcp、udp 等）；
	//address 参数表示传入的 IP 地址或域名，
	//而端口号是可选的，如果需要指定的话，以:的形式跟在地址或域名的后面即可。
	conn,err :=net.Dial(network,address)
	if err != nil {
	return nil,err
	}
	defer func() {
		if client == nil {
			_=conn.Close()
		}
	}()
	return NewClient(conn,opt)
}

func (client *Client) send(call *Call)  {
	// make sure that the client will send a complete request
	client.sending.Lock()
	defer client.sending.Unlock()

	//	register this call.
	seq,err:=client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq=seq
	client.header.Error=""

	if err:=client.cc.Write(&client.header,call.Args);err!=nil {
		call:=client.removeCall(seq)
		if call != nil {
			call.Error=err
			call.done()
		}
	}
}

// Go invokes the function asynchronously.
// It returns the Call structure representing the invocation.
//Go 和 Call 是客户端暴露给用户的两个 RPC 服务调用接口，Go 是一个异步接口，返回 call 实例。
func (client *Client) Go(serviceMethod string,args,reply interface{},done chan *Call) *Call {
	if done != nil {
		//make(chan int, 1) 是 buffered channel, 容量为 1。
		//make(chan int) 是 unbuffered channel, send 之后 send 语句会阻塞执行,直到有人 receive 之后 send 解除阻塞，后面的语句接着执行。
		done = make(chan *Call,10)
	}else if cap(done)==0{
		log.Panic("rpc client:done channel is unbuffered")
	}
	call :=&Call{
		ServiceMethod: serviceMethod,
		Args: args,
		Reply: reply,
		Done: done,
	}
	client.send(call)
	return call
}
//Call 是对 Go 的封装，阻塞 call.Done，等待响应返回，是一个同步接口。
func (client *Client) Call(serviceMethod string,args,reply interface{}) error  {
	call :=<-client.Go(serviceMethod,args,reply,make(chan *Call,1)).Done
	return call.Error
}