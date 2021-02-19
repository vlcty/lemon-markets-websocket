/*
MIT License

Copyright (c) 2021 Josef 'veloc1ty' Stautner

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package lemon provides easy access to the lemon.markets WebSocket streams
//
// They offer two streams: Ticks are price and quantity updates of trades and/or simple price updates for an instrument. Quotes are bid, ask, bidszie and asksizes updates for an instrument.
//
// An instrument is for example a security or a commodity and is identified by an International Securities Identification Number or short: ISIN
//
// Lang und Schwarz, Market Maker, Xetra, opening hours
//
// Lemon.markets is hooked up to Lang und Schwarz (L&S) Tradecenter, a market maker from germany. During the opening hours of Xetra, the digital exchange from the Frankfurt Stock Exchange, the spreads will not differ much. This is called "Referenzmarktprinzip".
//
// Current Xetra opening hours: https://www.xetra.com/xetra-en/trading/trading-calendar-and-trading-hours
// Currennt Lang und Schwarz opening hours: https://www.ls-tc.de/de/handelszeiten
//
// Connecting to lemon.markets outside L&S' opening hours is pointess. See function IsExchangeOpen for more details.
//
// Use of channels
//
// This library is using channels for the communication with your application. To be precise: It's using *your* channels. You are responsible for each channel! It's your decision if you use a buffered or unbuffered channel. It's your responsibility to open, close and empty them. Please make sure your receiver is fetching fast enough (< 10 seconds). Otherwise lemon.markets may close the stream.
//
// Disconnects
//
// Connection state is interally monitored. If the connection drops a reconnect is automatically performed.
package lemon

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// ErrConnectFailed is returned when the WebSocket connection failed
	ErrConnectFailed error = errors.New("Can't connect to lemon markets")

	// ErrConnectionClosed is returned when an active WebSocket connection closed. All further processing is stopped.
	ErrConnectionClosed error = errors.New("Lemon markets connection closed")

	// ErrUnknownISIN is returned when a subscription for an invalid or unknown ISIN occured. This error does not stop message processing
	ErrUnknownISIN error = errors.New("Invalid ISIN")

	// ErrInvalidRequest is returned when an invalud request was detected
	ErrInvalidRequest error = errors.New("Invalid request detected")

	// ErrNotImplemented is returned when the returned update did not match any type. This should never occure.
	ErrNotImplemented error = errors.New("Update type not implemented. You should never see this")
)

const (
	// Stream is initalizing
	State_init string = "initalizing"

	// Stream is connecting
	State_connecting string = "connecting"

	// Stream is connected
	State_connected string = "connected"

	// Stream is disconnected. This is a final state. It's only reached after calling Disconnect()
	State_disconnected string = "disconnected"

	// Stream is waiting to do a reconnect
	State_waiting_to_reconnect string = "waiting to reconnect"
)

type lemonMarketSubscription struct {
	Action    string `json:"action"`
	Specifier string `json:"specifier"`
	ISIN      string `json:"value"`
}

// Tick represents a price update.
type Tick struct {
	ISIN     string  `json:"isin"`     // ISIN of the instrument
	Price    float64 `json:"price"`    // Current market price
	Quantity uint    `json:"quantity"` // The quantity of the trade. If 0 then there was no actual trade but a simple price update
}

// Quote represents a quote update.
type Quote struct {
	ISIN    string  `json:"isin"`      // ISIN of the instrument
	Bid     float64 `json:"bid_price"` // Current bid price
	Ask     float64 `json:"ask_price"` // Current ask price
	Bidsize uint64  `json:"bid_quan"`  // Current bid size
	Asksize uint64  `json:"ask_quan"`  // Current ask size
}

// stream contains values, functions and channels shared by TickStream and QuoteStream
type stream struct {
	connection        *websocket.Conn
	processData       bool                                  // Read messages from the WebSocket
	subscriptions     map[string]uint                       // All subscriptions the user did
	getUpdateType     func() interface{}                    // Function returning the needed update type (tick or quote)
	sendUpdate        func(interface{})                     // Function to send the update into the channel
	getWebsocketUrl   func() string                         // Returns the websocket URL
	getSubscription   func(string) *lemonMarketSubscription // Creates a subscription type with the needed values
	reconnectNotifier chan uint                             // Channel to notify reconnectWatchdog to do a reconnect. Channel is under our control!
	failedReconnects  int
	state             string        // Current state
	errorChannel      chan<- error  // Channel where errors are sent into. Under user control!
	rawMessages       chan<- []byte // Channel where raw messages from the WebSocket are sent into if not nil. Under user control!
}

// init initalized shared variables and channels and start the reconnect watchdog
func (stream *stream) init() {
	stream.state = State_init
	stream.subscriptions = make(map[string]uint)
	stream.reconnectNotifier = make(chan uint, 1)
	stream.failedReconnects = 0

	go stream.reconnectWatchdog()
}

// reconnectWatchdog listens on the reconnectNotifier channel. Every time it pops something from it a reconnect to the
// WebSocket is needed
func (stream *stream) reconnectWatchdog() {
	for range stream.reconnectNotifier {
		stream.state = State_waiting_to_reconnect
		time.Sleep(time.Minute * time.Duration(stream.failedReconnects))

		stream.state = State_connecting
		stream.connect()
	}
}

// TickStream streams ticks for the subscribed securities
type TickStream struct {
	stream
	updateChannel chan<- *Tick
}

// QuoteStream streams quotes for the subscribed securities
type QuoteStream struct {
	stream
	updateChannel chan<- *Quote
}

// NewTickStream will initalize a new connection to stream ticks. Keep in mind: You are responsible for the passed
// channels.
func NewTickStream(updateChan chan<- *Tick, errChan chan<- error) *TickStream {
	stream := &TickStream{}
	stream.init()
	stream.errorChannel = errChan
	stream.updateChannel = updateChan

	stream.getUpdateType = func() interface{} {
		return &Tick{}
	}

	stream.sendUpdate = func(update interface{}) {
		stream.updateChannel <- update.(*Tick)
	}

	stream.getWebsocketUrl = func() string {
		return "wss://api.lemon.markets/streams/v1/marketdata"
	}

	stream.getSubscription = func(isin string) *lemonMarketSubscription {
		return &lemonMarketSubscription{
			ISIN:      isin,
			Action:    "subscribe",
			Specifier: "with-quantity-with-uncovered"}
	}

	stream.state = State_connecting

	stream.connect()

	return stream
}

// NewQuoteStream will initalize a new connection to stream quotes. Keep in mind: You are responsible for the passed
// channels.
func NewQuoteStream(updateChan chan<- *Quote, errChan chan<- error) *QuoteStream {
	stream := &QuoteStream{}
	stream.init()
	stream.errorChannel = errChan
	stream.updateChannel = updateChan

	stream.getUpdateType = func() interface{} {
		return &Quote{}
	}

	stream.sendUpdate = func(update interface{}) {
		stream.updateChannel <- update.(*Quote)
	}

	stream.getWebsocketUrl = func() string {
		return "wss://api.lemon.markets/streams/v1/quotes"
	}

	stream.getSubscription = func(isin string) *lemonMarketSubscription {
		return &lemonMarketSubscription{
			ISIN:      isin,
			Action:    "subscribe",
			Specifier: "with-quantity-with-price"}
	}

	stream.state = State_connecting

	stream.connect()

	return stream
}

func (lms *stream) sendSubscription(subscription *lemonMarketSubscription) {
	lms.connection.WriteJSON(subscription)
}

// Subscribe to an instrument by supplying an ISIN. Double subscriptions are prevented silently.
func (lms *stream) Subscribe(isin string) {
	if _, exists := lms.subscriptions[isin]; !exists {
		lms.subscriptions[isin] = 1
		lms.sendSubscription(lms.getSubscription(isin))
	}
}

// Unsubscribe to an instrument by supplying an ISIN. Double unsubscriptions are prevented silently.
func (lms *stream) Unsubscribe(isin string) {
	if _, exists := lms.subscriptions[isin]; exists {
		delete(lms.subscriptions, isin)

		lms.connection.WriteJSON(&lemonMarketSubscription{
			Action: "unsubscribe",
			ISIN:   isin})
	}
}

// GetState returns a human readable connection state. See constants for possible values.
func (lms *stream) GetState() string {
	return lms.state
}

// GetSubscriptions returns all stored subscriptions
func (lms *stream) GetSubscriptions() []string {
	subs := make([]string, 0)

	for isin, _ := range lms.subscriptions {
		subs = append(subs, isin)
	}

	return subs
}

// Disconnect will disconnect from the WebSocket and clean up
func (lms *stream) Disconnect() {
	lms.state = State_disconnected
	lms.processData = false
	lms.connection.Close()
	close(lms.reconnectNotifier)
}

func (lms *stream) connect() {
	connection, _, connectionError := websocket.DefaultDialer.Dial(lms.getWebsocketUrl(), nil)

	if connectionError != nil {
		lms.errorChannel <- ErrConnectFailed

		if lms.failedReconnects <= 5 {
			lms.failedReconnects++
		}

		lms.reconnectNotifier <- 1
	} else {
		lms.connection = connection
		lms.processData = true
		lms.failedReconnects = 0
		lms.state = State_connected

		go lms.listen()

		for isin, _ := range lms.subscriptions {
			lms.sendSubscription(lms.getSubscription(isin))
		}
	}
}

func (lms *stream) listen() {
	for lms.processData {
		_, msg, err := lms.connection.ReadMessage()

		if err != nil {
			lms.processData = false

			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseAbnormalClosure) {
				lms.errorChannel <- ErrConnectionClosed
			} else {
				lms.errorChannel <- err
			}

			if lms.state != State_disconnected {
				lms.reconnectNotifier <- 1
			}
		} else if isUnknownISIN(msg) {
			lms.errorChannel <- ErrUnknownISIN
		} else if isInvalidRequest(msg) {
			lms.errorChannel <- ErrInvalidRequest
		} else {
			if lms.rawMessages != nil {
				lms.rawMessages <- msg
			}

			update := lms.getUpdateType()
			var decodeError error

			switch update.(type) {
			case *Tick:
				decodeError = json.Unmarshal(msg, update.(*Tick))

			case *Quote:
				decodeError = json.Unmarshal(msg, update.(*Quote))

			default:
				decodeError = ErrNotImplemented
			}

			if decodeError != nil {
				lms.errorChannel <- decodeError
			} else {
				lms.sendUpdate(update)
			}
		}
	}
}

// SetRawMessageChannel will take a channel where raw, untouched messages from the WebSocket will be sent into.
// Keep in mind that you are the one in charge of maintaining and servicing the channel.
func (lms *stream) SetRawMessageChannel(channel chan<- []byte) {
	lms.rawMessages = channel
}

func isUnknownISIN(message []byte) bool {
	return strings.Contains(string(message), "This instrument does not exist")
}

func isInvalidRequest(message []byte) bool {
	return strings.Contains(string(message), "Invalid request")
}

func isExchangeOpen(now time.Time) bool {
	location, _ := time.LoadLocation("Europe/Berlin")

	openingHours := map[time.Weekday][4]int{
		time.Saturday: [4]int{10, 0, 13, 0}, // 10:00 - 13:00
		time.Sunday:   [4]int{17, 0, 19, 0}, // 17:00 - 19:00
	}

	var opening, closing time.Time

	if hours, exists := openingHours[now.Weekday()]; exists {
		opening = time.Date(now.Year(), now.Month(), now.Day(), hours[0], hours[1], 0, 0, location)
		closing = time.Date(now.Year(), now.Month(), now.Day(), hours[2], hours[3], 0, 0, location)
	} else {
		opening = time.Date(now.Year(), now.Month(), now.Day(), 7, 30, 0, 0, location)
		closing = time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, location)
	}

	return now.After(opening) && now.Before(closing)
}

// IsExchangeOpen returns true if Lang und Schwarz Tradecenter is currently operating.
func IsExchangeOpen() bool {
	location, _ := time.LoadLocation("Europe/Berlin")
	return isExchangeOpen(time.Now().In(location))
}
