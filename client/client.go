package client

import (
	"fmt"
	"net/http"

	"golang.org/x/net/websocket"
)

type (
	// OnConnected will be called when the client connects to the server.
	OnConnected func()

	// OnDisconnected will be called when the client disconnects from the server.
	OnDisconnected func()

	// OnErr will be called when the client encounters an error.
	OnErr func(error)

	// OnMessageReceived will be called when the client receives a message from the server.
	OnMessageReceived func(msg []byte, r *http.Request)

	// OnSendMessage will be called when a message is sent to the server.
	OnSendMessage func(msg []byte, r *http.Request)

	// Client is the base type of this library. It will handle the connection with the server
	// along with sending and receiving messages.
	Client struct {
		connected         bool
		connection        *websocket.Conn
		onConnected       OnConnected
		onDisconnected    OnDisconnected
		onErr             OnErr
		onMessageReceived OnMessageReceived
		onSendMessage     OnSendMessage
	}
)

// Connect to the server.
func (c *Client) Connect(address string) {
	ws, err := websocket.Dial(fmt.Sprintf("ws://%s/ws", address), "", fmt.Sprintf("http://%s/", address))
	if err != nil {

	}

	c.connection = ws
}

// New returns an instantiated client.
func New(oc OnConnected, od OnDisconnected, oe OnErr, omr OnMessageReceived, osm OnSendMessage) *Client {
	return &Client{
		connected:         false,
		onConnected:       oc,
		onDisconnected:    od,
		onErr:             oe,
		onMessageReceived: omr,
		onSendMessage:     osm,
	}
}
