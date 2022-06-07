package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	//conn 是由构建函数传入，通常是通过 TCP 或者 Unix 建立 socket 时得到的链接实例
	conn io.ReadWriteCloser
	//buf 是为了防止阻塞而创建的带缓冲的 Writer
	buf *bufio.Writer
	//dec 和 enc 对应 gob 的 Decoder 和 Encoder
	//gob是Golang包自带的一个数据结构序列化的编码/解码工具
	//编码使用Encoder，解码使用Decoder
	dec *gob.Decoder
	enc *gob.Encoder
}

var _ Codec = (*GobCodec)(nil)

//接着实现 ReadHeader、ReadBody、Write 和 Close 方法。

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf:=bufio.NewWriter(conn)
	return &GobCodec{
		conn:conn,
		buf:buf,
		//相对于解码，json.NewEncoder进行大JSON的编码比json.marshal性能高，因为内部使用pool。
		dec:gob.NewDecoder(conn),
		enc: gob.NewEncoder(buf),
	}
}
func (c *GobCodec) Close() error {
	return c.conn.Close()
}

func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_=c.buf.Flush()
		if err != nil {
			_=c.Close()
		}
	}()
	if err=c.enc.Encode(h); err != nil {
		log.Println("rpc:gob error encoding header:",err)
		return
	}
	if err=c.enc.Encode(body); err != nil {
		log.Println("rpc:gob error encoding header:",err)
		return
	}
	return
}

