package gosocketio

import (
	"github.com/integration-system/golang-socketio/transport"
	"strconv"
	"sync"
	"time"
)

const (
	webSocketProtocol       = "ws://"
	webSocketSecureProtocol = "wss://"
	socketioUrl             = "/socket.io/?EIO=3&transport=websocket"
)

const (
	defaultReconnectionTimeout = 3 * time.Second
)

/**
Socket.io client representation
*/
type Client struct {
	methods
	Channel
	tr               transport.Transport
	rp               *ReconnectionPolicy
	reconnectChannel chan bool
	url              string
	lock             sync.Mutex
	open             bool
}

type ReconnectionPolicy struct {
	Enable              bool
	ReconnectionTimeout time.Duration
	OnReconnectionError func(err error)
}

type clientBuilder struct {
	client *Client
}

func (cb *clientBuilder) Transport(tr transport.Transport) *clientBuilder {
	cb.client.tr = tr
	return cb
}

func (cb *clientBuilder) EnableReconnection() *clientBuilder {
	cb.client.rp.Enable = true
	return cb
}

func (cb *clientBuilder) ReconnectionTimeout(timeout time.Duration) *clientBuilder {
	cb.client.rp.ReconnectionTimeout = timeout
	return cb
}

func (cb *clientBuilder) OnReconnectionError(handler func(err error)) *clientBuilder {
	cb.client.rp.OnReconnectionError = handler
	return cb
}

func (cb *clientBuilder) On(event string, f interface{}, onSubError func(event string, err error)) *clientBuilder {
	if err := cb.client.On(event, f); err != nil && onSubError != nil {
		onSubError(event, err)
	}
	return cb
}

func (cb *clientBuilder) UnsafeClient() *Client {
	return cb.client
}

func (cb *clientBuilder) BuildToConnect(targetUrl string) *Client {
	cb.client.initChannel()
	if cb.client.rp.Enable {
		cb.client.runReconnectionTask()
	}
	cb.client.url = targetUrl
	return cb.client
}

func NewClientBuilder() *clientBuilder {
	c := &Client{
		tr: transport.GetDefaultWebsocketTransport(),
		rp: &ReconnectionPolicy{
			Enable: false,
		},
	}
	c.initMethods()
	return &clientBuilder{client: c}
}

/**
connect to host and initialise socket.io protocol

The correct ws protocol url example:
ws://myserver.com/socket.io/?EIO=3&transport=websocket

You can use GetUrlByHost for generating correct url
*/
func (c *Client) Dial() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	var err error
	c.conn, err = c.tr.Connect(c.url)
	if err != nil {
		return err
	}

	c.SetAlive(true)

	go inLoop(&c.Channel, &c.methods)
	go outLoop(&c.Channel, &c.methods)
	go pinger(&c.Channel)

	c.open = true

	return nil
}

/**
Close client connection
*/
func (c *Client) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.open {
		c.open = false
		close(c.reconnectChannel)
		closeChannel(&c.Channel, &c.methods)
	}
}

func (c *Client) runReconnectionTask() {
	c.reconnectChannel = make(chan bool)
	c.onDisconnection = func(channel *Channel) {
		if c.open {
			c.reconnectChannel <- true
		}
	}
	go func() {
		for {
			if _, open := <-c.reconnectChannel; !open {
				return
			}
			connected := false
			for !connected {
				timeout := c.rp.ReconnectionTimeout
				if timeout <= 0 {
					timeout = defaultReconnectionTimeout
				}
				time.Sleep(timeout)
				err := c.Dial()
				if err != nil {
					if c.rp.OnReconnectionError != nil {
						c.rp.OnReconnectionError(err)
					}
				} else {
					connected = true
				}
			}
		}
	}()
}

/**
Get ws/wss url by host and port
*/
func GetUrl(host string, port int, secure bool, params map[string]string) string {
	var prefix string
	if secure {
		prefix = webSocketSecureProtocol
	} else {
		prefix = webSocketProtocol
	}
	connectionString := prefix + host + ":" + strconv.Itoa(port) + socketioUrl
	if len(params) > 0 {
		for k, v := range params {
			connectionString += "&" + k + "=" + v
		}
	}
	return connectionString
}
