package minirpc

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"minirpc/codec"
	"net"
	"reflect"
	"strings"
	"sync"
)

const MagicNumber =0x3bef5c
// HTTP 报文，分为 header 和 body 2 部分，
//body 的格式和长度通过 header 中的 Content-Type 和 Content-Length 指定，
//服务端通过解析 header 就能够知道如何从 body 中读取需要的信息。
//| Option{MagicNumber: xxx, CodecType: xxx}
//| <-------   编码方式由 CodeType 决定   ------->|
//| Header{ServiceMethod ...} | Body interface{} |
//| <------      固定 JSON 编码      ------>

type Option struct {
	MagicNumber int  // MagicNumber marks this's a minirpc request
	CodecType codec.Type // client may choose different Codec to encode body
}

type Server struct {
	serviceMap sync.Map
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

//实现了 Accept 方式，net.Listener 作为参数，
//for 循环等待 socket 连接建立 ，并开启子协程处理，
//处理过程交给了 ServerConn 方法。
//DefaultServer 是一个默认的 Server 实例，

func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()
//后续的 header 和 body 的编码方式由 Option 中的 CodeType 指定，
//服务端首先使用 JSON 解码 Option，然后通过 Option 的 CodeType 解码剩余的内容。

func (server *Server) ServeConn(conn io.ReadWriteCloser)  {
	defer func() {
		_=conn.Close()
	}()
	var opt Option
	if err:=json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server:options error: ",err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server:invalid codec number %x",opt.CodecType)
		return
	}
	f:=codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server:invalid codec type %s",opt.CodecType)
		return
	}
	server.serveCodec(f(conn))
}
// invalidRequest is a placeholder for response argv when error occurs
var invalidRequest = struct {}{}
func (server *Server) serveCodec(cc codec.Codec) {
	sending :=new(sync.Mutex)
	wg:=new(sync.WaitGroup)
	for  {
		req,err:=server.readRequest(cc)
		if err != nil {
			if req ==nil{
				break
			}
			req.h.Error = err.Error()
			server.sendResponse(cc,req.h, invalidRequest,sending)
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc,req,sending,wg)
	}
}

type request struct {
	h *codec.Header
	argv,replyv reflect.Value
	mtype *methodType
	svc *service
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header,error) {
	var h codec.Header
	if err:=cc.ReadHeader(&h); err != nil {
		if err!=io.EOF && err!=io.ErrUnexpectedEOF{
			log.Println("rpc server:read header error:",err)
		}
		return nil,err
	}
	return &h,nil
}

func (server *Server) readRequest(cc codec.Codec) (*request,error) {
	h,err:=server.readRequestHeader(cc)
	if err != nil {
		return nil,err
	}
	req:=&request{h: h}
	// TODO: now we don't know the type of request argv
	req.svc,req.mtype,err = server.findService(h.ServiceMethod)
	if err!=nil {
		return req,err
	}
	//通过 newArgv() 和 newReplyv() 两个方法创建出两个入参实例，
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()
	// day 1, just suppose it's string
	argvi := req.argv.Interface()
	//通过 cc.ReadBody() 将请求报文反序列化为第一个入参 argv
	//在这里同样需要注意 argv 可能是值类型，也可能是指针类型，
	if req.argv.Type().Kind()!=reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err =cc.ReadBody(argvi);err!=nil {
		log.Println("rpc server:read body err:",err)
		return req,nil
	}
	return req,nil
}

//sync包和channel机制来解决并发机制中不同goroutine之间的同步和通信
func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	//使用延迟执行语句在函数退出时释放资源
	defer sending.Unlock()
	if err:=cc.Write(h,body) ;err != nil {
		log.Println("rpc server:write response error:",err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	// TODO, should call registered rpc methods to get the right replyv
	// day 1, just print argv and send a hello message
	defer wg.Done()
	//通过 req.svc.call 完成方法调用，将 replyv 传递给 sendResponse 完成序列化即可。
	err:=req.svc.call(req.mtype,req.argv,req.replyv)
	if err != nil {
		req.h.Error = err.Error()
		server.sendResponse(cc, req.h, invalidRequest, sending)
		return
	}
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}
//实现了 Accept 方式，net.Listener 作为参数，
//for 循环等待 socket 连接建立，
//并开启子协程处理，处理过程交给了 ServerConn 方法

func (server *Server) Accept(lis net.Listener)  {
	for  {
		conn,err:=lis.Accept()
		if err != nil {
			log.Println("rpc server:accept error:",err)
			return
		}
		go server.ServeConn(conn)
	}
}
//DefaultServer 是一个默认的 Server 实例，主要为了用户使用方便。
// Server represents an RPC Server.

func Accept(lis net.Listener)  {
	DefaultServer.Accept(lis)
}

// Register publishes in the server the set of methods of the
func (server *Server) Register(rcvr interface{}) error  {
	s:=newService(rcvr)
	if _,dup:=server.serviceMap.LoadOrStore(s.name,s);dup {
		return errors.New("rpc: service already defined:"+s.name)
	}
	return nil
}

// Register publishes the receiver's methods in the DefaultServer.
func Register(rcvr interface{}) error   {
	return DefaultServer.Register(rcvr)
}

//通过 ServiceMethod 从 serviceMap 中找到对应的 service
func (server *Server) findService(serviceMethod string)(svc *service,mtype *methodType, err error)  {
	// ServiceMethod 的构成是 “Service.Method”
	dot:=strings.LastIndex(serviceMethod,".")
	if dot<0{
		err = errors.New("rpc server:service/method request ill-formed:"+serviceMethod)
		return
	}
	//第一部分是 Service 的名称，第二部分即方法名
	serviceName,methodName := serviceMethod[:dot],serviceMethod[dot+1:]
	svci,ok :=server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server:can't find service"+serviceName)
		return
	}
	svc = svci.(*service)
	mtype = svc.method[methodName]
	if mtype==nil {
		err=errors.New("rpc server : can't find method" + methodName)
	}
	return
}