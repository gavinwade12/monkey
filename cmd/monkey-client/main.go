package main

import (
	"log"
	"net/http"

	"github.com/gavinwade12/monkey/client"
)

type chatHandler struct{}

func (c *chatHandler) Connected() {}

func (c *chatHandler) Disconnected() {}

func (c *chatHandler) Err(err error) { log.Print(err) }

func (c *chatHandler) MessageReceived(msg []byte, r *http.Request) {}

func (c *chatHandler) SendMessage(msg []byte, r *http.Request) {}

func main() {
	h := &chatHandler{}
	oc := client.OnConnected(h.Connected)
	od := client.OnDisconnected(h.Disconnected)
	oe := client.OnErr(h.Err)
	omr := client.OnMessageReceived(h.MessageReceived)
	osm := client.OnSendMessage(h.SendMessage)

	c := client.New(oc, od, oe, omr, osm)
	c.Connect("")
}
