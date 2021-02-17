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
// Lang und Schwarz, Market Maker, Xetra
//
// Lemon.markets is hooked up to Lang und Schwarz Tradecenter, a market maker from germany. During business hours of Xetra, the digital exchange from the Frankfurt Stock Exchange, the spreads will not differ much. This is called "Referenzmarktprinzip". Xetra's business hours are currently Monday to Friday 09:00 until 17:30 CET.
//
// Currently Lang und Schwarz offers these opening hours:
//         Monday to Friday: 07:30 until 23:00 CET
//         Saturday: 10:00 until 13:00 CET
//         Sunday: 17:00 until 19:00 CET
//
// It's no use to connect to lemon.markets outside the business hours. Also the connection to lemon.markets is automatically terminated after one hour. Design your application to withstand that!
//
// More about Xetra opening hours: https://www.xetra.com/xetra-en/trading/trading-calendar-and-trading-hours
// More about Lang und Schwarz opening hours: https://www.ls-tc.de/de/handelszeiten
//
// Use of channels
//
// This library is using channels for the communication with your application. To be precise: It's using *your* channels. You are responsible for each channel! It's your decision if you use a buffered or unbuffered channel. It's your responsibility to open, close and empty them. Please make sure your receiver is fetching fast enough (< 20 seconds). Otherwise lemon.markets may close the stream.
package lemon

import (
	"encoding/json"
	"errors"
	"strings"

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

type lemonMarketSubscription struct {
	Action    string `json:"action"`
	Specifier string `json:"specifier"`
	ISIN      string `json:"value"`
}

// Tick represents a price update.m
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

type stream struct {
	connection      *websocket.Conn
	processData     bool
	subscriptions   map[string]uint
	getUpdateType   func() interface{}
	sendUpdate      func(interface{})
	getWebsocketUrl func() string
	getSubscription func() *lemonMarketSubscription

	errorChannel chan<- error
	rawMessages  chan<- []byte
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

// NewTickStream will initalize a new connection to stream ticks. Keep in mind: You are responsible for the passed channels. Raises an error if the connection fails.
func NewTickStream(updateChan chan<- *Tick, errChan chan<- error) (*TickStream, error) {
	stream := &TickStream{}
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

	stream.getSubscription = func() *lemonMarketSubscription {
		return &lemonMarketSubscription{
			Action:    "subscribe",
			Specifier: "with-quantity-with-uncovered"}
	}

	err := stream.connect()

	return stream, err
}

// NewQuoteStream will initalize a new connection to stream quotes. Keep in mind: You are responsible for the passed channels. Raises an error if the connection fails.
func NewQuoteStream(updateChan chan<- *Quote, errChan chan<- error) (*QuoteStream, error) {
	stream := &QuoteStream{}
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

	stream.getSubscription = func() *lemonMarketSubscription {
		return &lemonMarketSubscription{
			Action:    "subscribe",
			Specifier: "with-quantity-with-price"}
	}

	err := stream.connect()

	return stream, err
}

// Subscribe to an instrument by supplying an ISIN. Double subscriptions are prevented silently.
func (lms *stream) Subscribe(isin string) {
	if _, exists := lms.subscriptions[isin]; !exists {
		lms.subscriptions[isin] = 1

		subscription := lms.getSubscription()
		subscription.ISIN = isin

		lms.connection.WriteJSON(subscription)
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

func (lms *stream) connect() error {
	connection, _, connectionError := websocket.DefaultDialer.Dial(lms.getWebsocketUrl(), nil)

	if connectionError != nil {
		return ErrConnectFailed
	} else {
		lms.connection = connection
		lms.processData = true
		lms.subscriptions = make(map[string]uint)

		go lms.listen()

		return nil
	}
}

// Disconnect will disconnect from the WebSocket. Disconnect will not raise an error when the connection was already terminated.
func (lms *stream) Disconnect() {
	lms.processData = false
	lms.connection.Close()
}

func (lms *stream) listen() {
	for {
		update := lms.getUpdateType()

		_, msg, err := lms.connection.ReadMessage()

		if !lms.processData {
			// While waiting for a message the connection cosed or user does not want to continue. Exit this routine!
			return
		} else if err != nil {
			lms.processData = false

			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseAbnormalClosure) {
				lms.errorChannel <- ErrConnectionClosed
			} else {
				lms.errorChannel <- err
			}
		} else if isUnknownISIN(msg) {
			lms.errorChannel <- ErrUnknownISIN
		} else if isInvalidRequest(msg) {
			lms.errorChannel <- ErrInvalidRequest
		} else {
			if lms.rawMessages != nil {
				lms.rawMessages <- msg
			}

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

// SetRawMessageChannel will take a channel where raw, untouched messages from the WebSocket will be sent into. Keep in mind that you are the one in charge of maintaining the channel.
func (lms *stream) SetRawMessageChannel(channel chan<- []byte) {
	lms.rawMessages = channel
}

func isUnknownISIN(message []byte) bool {
	return strings.Contains(string(message), "This instrument does not exist")
}

func isInvalidRequest(message []byte) bool {
	return strings.Contains(string(message), "Invalid request")
}
