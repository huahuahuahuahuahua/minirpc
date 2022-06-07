package minirpc

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)
//反射是指在程序运行期对程序本身进行访问和修改的能力。
type methodType struct {
	method reflect.Method //方法本身
	ArgType	reflect.Type //第一个参数的类型
	ReplyType reflect.Type //第二个参数的类型
	numCalls uint64 //方法调用次数
}

//接收者。这里是定义他们的方法有两种。如果你想修改接收器
//如果你不需要修改接收器，则可以将接收器定义为如下值：
//func (s MyStruct)  valueMethod()   { } // method on value

func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}
//newArgv() 和 newReplyv() 两个方法创建出两个入参实例，
//然后通过 cc.ReadBody() 将请求报文反序列化为第一个入参 argv，
//在这里同样需要注意 argv 可能是值类型，也可能是指针类型
func (m *methodType) newArgv() reflect.Value {
	var argv reflect.Value
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
	}else {
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

func (m methodType) newReplyv() reflect.Value  {
	//reply must be a pointer type
	//reflect.New()函数用于获取表示指向新零值的指针的Value指定的类型。
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(),0,0))
	}
	return replyv
}
//反射的定义是这样的：在编译时不知道类型的情况下，可更新变量，在运行时查看值，调用方法以及直接对它们的布局进行操作。
type service struct {
	name string
	typ reflect.Type
	rcvr reflect.Value
	method map[string]*methodType
}

//保留 rcvr 是因为在调用时需要 rcvr 作为第 0 个参数；method 是 map 类型，存储映射的结构体的所有符合条件的方法
//rcvr 即结构体的实例本身
func newService(rcvr interface{}) *service {
	s :=new(service)
	s.rcvr = reflect.ValueOf(rcvr)
	// name返回其包中的类型名称，举个例子，这里会返回Person，tool
	s.name = reflect.Indirect(s.rcvr).Type().Name()
	s.typ =reflect.TypeOf(rcvr)
	if !ast.IsExported(s.name){
		log.Fatalf("rpc server:%s is not a vaild service name",s.name)
	}
	s.registerMethods()
	return s
}
//registerMethods 过滤出了符合条件的方法：
func (s *service) registerMethods() {
	s.method = make(map[string]*methodType)
	for i:=0;i<s.typ.NumMethod();i++ {
	method :=s.typ.Method(i)
	mType := method.Type
		//两个导出或内置类型的入参（反射时为 3 个，第 0 个是自身，类似于 python 的 self，java 中的 this）
		//返回值有且只有 1 个，类型为 error
		if mType.NumIn()!=3||mType.NumOut()!=1 {
			continue
		}
		if mType.Out(0)!=reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name]=&methodType{
			method:method,
			ArgType: argType,
			ReplyType: replyType,
		}
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	//sExported 报告名称是否为导出的 Go 符号（即，它是否以大写字母开头）。 
	return ast.IsExported(t.Name()) ||t.PkgPath()==""
}
//能够通过反射值调用方法。
func (s *service) call(m *methodType,argv,replgv reflect.Value) error {
	//addr表示地址，而delta表示少量大于零的位
	atomic.AddUint64(&m.numCalls,1)
	f:=m.method.Func
	returnValues :=f.Call([]reflect.Value{s.rcvr,argv,replgv})
	if errInter:=returnValues[0].Interface();errInter!=nil {
		return errInter.(error)
	}
	return nil
}

