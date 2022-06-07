package codec

import "io"

type Header struct {
	ServiceMethod string //ServiceMethod 是服务名和方法名，通常与 Go 语言中的结构体和方法相映射。
	Seq 	      uint64 //Seq 是请求的序号，也可以认为是某个请求的 ID，用来区分不同的请求。
	Error 		  string //Error 是错误信息，客户端置为空，服务端如果如果发生错误，将错误信息置于 Error 中。
}

type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{})error
	Write(*Header,interface{})error
}

type NewCodecFunc func(io.ReadWriteCloser) Codec
type Type string

//Gob 和 Json，但是实际代码中只实现了 Gob 一种
const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json" // not implemented
)

var NewCodecFuncMap map[Type]NewCodecFunc
func init()  {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
	NewCodecFuncMap[JsonType] = NewGobCodec
}