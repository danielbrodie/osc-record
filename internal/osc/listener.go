package osc

import (
	"fmt"
	"net"
	"strings"

	goosc "github.com/hypebeast/go-osc/osc"
)

// Handler is called when an OSC message arrives.
type Handler func(addr string, args []interface{})

type Message struct {
	Address   string
	Arguments []interface{}
}

// Server wraps go-osc for osc-record's needs.
type Server struct {
	port    int
	server  *goosc.Server
	conn    net.PacketConn
	handler Handler
}

type Listener struct {
	server *Server
}

func NewServer(port int, handler Handler) *Server {
	return &Server{port: port, handler: handler}
}

func (s *Server) ListenAndServe() error {
	dispatcher := goosc.NewStandardDispatcher()
	if err := dispatcher.AddMsgHandler("*", func(msg *goosc.Message) {
		if s.handler != nil {
			s.handler(msg.Address, msg.Arguments)
		}
	}); err != nil {
		return err
	}

	s.server = &goosc.Server{
		Addr:       fmt.Sprintf(":%d", s.port),
		Dispatcher: dispatcher,
	}

	conn, err := net.ListenPacket("udp", s.server.Addr)
	if err != nil {
		return bindError(s.port, err)
	}
	s.conn = conn

	if err := s.server.Serve(conn); err != nil {
		return bindError(s.port, err)
	}
	return nil
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	if s.conn != nil {
		_ = s.conn.Close()
		return
	}
	if s.server != nil {
		_ = s.server.CloseConnection()
	}
}

func Listen(port int, handler func(Message)) (*Listener, error) {
	srv := NewServer(port, func(addr string, args []interface{}) {
		if handler != nil {
			handler(Message{Address: addr, Arguments: args})
		}
	})
	go func() {
		_ = srv.ListenAndServe()
	}()
	return &Listener{server: srv}, nil
}

func (l *Listener) Close() error {
	if l == nil || l.server == nil {
		return nil
	}
	l.server.Close()
	return nil
}

func bindError(port int, err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
		return fmt.Errorf("Error: Could not bind to port %d: address already in use. Use --port to specify a different port.", port)
	}
	return err
}
