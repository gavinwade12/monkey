package server

import (
	"errors"
	"net/http"
	"sync"

	uuid "github.com/satori/go.uuid"

	"golang.org/x/net/websocket"
)

type (
	// OnDispatch will be called when a message from a client is to be dispatched.
	// The original message will be provided along with the ID of the sender.
	// The function should return a slice of ClientIDs to broadcast the messages to,
	// as well as a possibly altered message to broadcast.
	OnDispatch func(msg []byte, senderID string) ([]string, []byte)
	// OnErr will be called when the server encounters an error.
	OnErr func(error)
	// OnNewClient is called when a new client is connecting. The ClientID and Request
	// are provided so external external bookkeeping can be done with the clients.
	// The function should return true if it's OK to connect the client, otherwise
	// the connection will be closed.
	OnNewClient func(id string, r *http.Request) bool

	// Server is the base type of this library. It does the low-level bookkeeping
	// of clients and dispatches messages and errors.
	Server struct {
		done        chan bool
		onDispatch  OnDispatch
		onErr       OnErr
		onNewClient OnNewClient
		mu          *sync.RWMutex
		messengers  map[string]*messenger
	}
)

var (
	// ErrClientNotFound signifies that a message was attempted to be dispatched
	// to a client that is no longer (or was never) connected.
	ErrClientNotFound = errors.New("monkey: client not found for broadcast")
	// ErrNilConn signifies that a client tried to connect, but the connection
	// received by the server was nil.
	ErrNilConn = errors.New("monkey: nil connection")
	// ErrNonServerConn signifies that the server received a connection from the client,
	// but for some reason it was not a server connection.
	ErrNonServerConn = errors.New("monkey: not a server connection")
)

// New returns an instantiated server.
func New(od OnDispatch, oe OnErr, onc OnNewClient) *Server {
	return &Server{
		done:        make(chan bool),
		onDispatch:  od,
		onErr:       oe,
		onNewClient: onc,
		mu:          &sync.RWMutex{},
		messengers:  make(map[string]*messenger),
	}
}

func (s *Server) add(m *messenger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messengers[m.id()] = m
}

// Dispatch calls the OnDispatch function to get the message to dispatch and
// the IDs of the clients to dispatch it to. It then sends the message to all
// the clients found.
func (s *Server) dispatch(msg []byte, senderID string) {
	ids, msg := s.onDispatch(msg, senderID)
	var sendTo []*messenger

	s.mu.RLock()
	for _, m := range s.messengers {
		var c *messenger
		for _, id := range ids {
			if m.clientID == id {
				c = m
				break
			}
		}
		if c == nil {
			s.err(ErrClientNotFound)
			continue
		}
		sendTo = append(sendTo, c)
	}
	s.mu.RUnlock()

	for _, m := range sendTo {
		m.send(msg)
	}
}

// Err passes the error to the server's OnError function.
func (s *Server) err(err error) {
	s.onErr(err)
}

// Remove the client from the server.
func (s *Server) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.messengers[id]; m != nil {
		m.done()
	}
	delete(s.messengers, id)
}

// SetOnNewClient sets a new function to be called when a new client connects.
// This could be used to limit the number of clients connected to the server.
func (s *Server) SetOnNewClient(onc OnNewClient) {
	s.onNewClient = onc
}

// Start starts the server.
func (s *Server) Start(addr string) {
	http.Handle(addr, websocket.Handler(s.newClientHandler))
}

func (s *Server) newClientHandler(conn *websocket.Conn) {
	if conn == nil {
		s.err(ErrNilConn)
		return
	} else if !conn.IsServerConn() {
		s.err(ErrNonServerConn)
		return
	}

	id := uuid.NewV4().String()
	if !s.onNewClient(id, conn.Request()) {
		if err := conn.Close(); err != nil {
			s.err(err)
		}
		return
	}

	m := newMessenger(id, conn, s)
	s.add(m)
	m.start()
}
