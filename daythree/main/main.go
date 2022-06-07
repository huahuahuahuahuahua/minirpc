package main

import (
	"log"
	"minirpc"
	"net"
	"sync"
	"time"
)

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer (addr chan string){
	var foo Foo
	if err:=minirpc.Register(&foo);err != nil {
		log.Fatal("register error:",err)
	}
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
	client,_:=minirpc.Dial("tcp",<-addr)
	defer func() {_=client.Close()}()

	time.Sleep(time.Second)
	//wg.Add(delta) 来改变值 wg 维护的计数
	//一个 WaitGroup 对象可以等待一组协程结束。Coroutines
	//一个线程可以有多个协程。Coroutines
	// 一个线程内的多个协程的运行是串行的，
	//串行通信。. 串行通信是指 使用一条数据线，将数据一位一位地依次传输
	//这点和多进程（多线程）在多核CPU上执行时是不同的。
	//多进程（多线程）在多核CPU上是可以并行的。当
	//线程内的某一个协程运行时，其它协程必须挂起。

	//并行情况，又N个线程，去分别执行N个任务。
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args :=&Args{Num1: i,Num2: i*i}
			var reply int
			if err:=client.Call("Foo.Sum",args,&reply);err != nil {
				log.Fatal("call Foo.SUm error:",err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
		wg.Wait()
	}
}