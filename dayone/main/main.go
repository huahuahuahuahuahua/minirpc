package main
//当导入一个包时，该包下的文件里所有init()函数都会被执行，
//有些时候我们并不需要把整个包都导入进来，仅仅是是希望它执行init()函数而已。
//这个时候就可以使用 import _ " "引用该包。

import (
	"minirpc"
	"encoding/json"
	"fmt"
	"log"
	"minirpc/codec"
	"net"
	"time"
)

func startServer (addr chan string){
	l,err := net.Listen("tcp",":0")
	if err != nil {
		//退出应用程序
		//defer函数不会执行
		log.Fatal("network error:",err)
	}
	log.Println("start rpc server on ",l.Addr())
	addr <- l.Addr().String()
	minirpc.Accept(l)
}

//在 startServer 中使用了信道 addr，确保服务端端口监听成功，客户端再发起请求。
//客户端首先发送 Option 进行协议交换，
//接下来发送消息头 h := &codec.Header{}，和消息体 minirpc req ${h.Seq}。

func main()  {
	// log.SetFlags() 函数在日志消息中添加完整的程序文件作为前缀
	log.SetFlags(0)
	addr:=make(chan string)
	go startServer(addr)
	// in fact, following code is like a simple rpc client
	//dial只需要直接填入连接地址
	conn,_:=net.Dial("tcp",<-addr)
	defer func() {_=conn.Close()}()

	time.Sleep(time.Second)
	_=json.NewEncoder(conn).Encode(minirpc.DefaultOption)
	cc:=codec.NewGobCodec(conn)
	for i := 0; i < 5; i++ {
		//发送消息头 h := &codec.Header{}，和消息体 minirpc req ${h.Seq}。
		h:=&codec.Header{
			ServiceMethod:"Foo.Sum",
			Seq: uint64(i),
		}
		//发送格式化输出到 str 所指向的字符串
		_ = cc.Write(h,fmt.Sprintf("minirpc req %d",h.Seq))
		_=cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply",reply)
	}
}