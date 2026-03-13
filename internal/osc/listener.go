package osc

import (
	"fmt"
	"net"
	"strings"

	goosc "github.com/hypebeast/go-osc/osc"
)

type Message struct {
	Address   string
	Arguments []interface{}
}

type Listener struct {
	conn   net.PacketConn
	server *goosc.Server
}

func Listen(port int, handler func(Message)) (*Listener, error) {
	dispatcher := goosc.NewStandardDispatcher()
	if err := dispatcher.AddMsgHandler("*", func(msg *goosc.Message) {
		if handler == nil {
			return
		}
		handler(Message{
			Address:   msg.Address,
			Arguments: msg.Arguments,
		})
	}); err != nil {
		return nil, err
	}

	server := &goosc.Server{
		Addr:       fmt.Sprintf(":%d", port),
		Dispatcher: dispatcher,
	}

	conn, err := net.ListenPacket("udp", server.Addr)
	if err != nil {
		return nil, bindError(port, err)
	}

	listener := &Listener{
		conn:   conn,
		server: server,
	}

	go func() {
		_ = server.Serve(conn)
	}()

	return listener, nil
}

func (l *Listener) Close() error {
	if l == nil || l.conn == nil {
		return nil
	}
	return l.conn.Close()
}

func bindError(port int, err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
		return fmt.Errorf("Error: Could not bind to port %d: address already in use. Use --port to specify a different port.", port)
	}
	return err
}
