# lemon-markets-websocket

A go wrapper lib for the [lemon.markets](https://lemon.markets) WebSocket streams.

## Installation

Install using go get:

```
go get -u github.com/vlcty/lemon-markets-websocket
```

## Usage

Lemon markets provides two streams:

- Price updates (ticks) represented by `lemon.Tick`
- Quotes (Bid, Ask, Sizes, etc) represented by `lemon.Quote`

The lib itself uses channels to communicate with your application. You are responsible for the channels to be created, sized and closed after usage. Message parsing may hang if the library needs to wait for your consumer.

Example code:

```go
package main

import (
	"fmt"
	"time"

	"github.com/vlcty/lemon-markets-websocket"
)

func main() {
	// Create the needed channels
	tickChan := make(chan *lemon.Tick, 0)
	quoteChan := make(chan *lemon.Quote, 0)
	errChan := make(chan error, 0)

	// Create a stream struct for tick updates and pass in the needed channels
	tickStream, tickErr := lemon.NewTickStream(tickChan, errChan)

	if tickErr != nil {
		panic(tickErr)
	}

	// Create a stream struct for quote updates and pass in the needed channels
	quoteStream, quoteErr := lemon.NewQuoteStream(quoteChan, errChan)

	if quoteErr != nil {
		panic(quoteErr)
	}

	// Define some ISINs
	isins := []string{
		"DE000TUAG000", // TUI AG
		"LS000IGOLD01", // L&S gold commodity
		"US00165C1045", // AMC entertainment
	}

	for _, isin := range isins {
		// Subscribe to ticks
		tickStream.Subscribe(isin)

		// Subscribe to quotes
		quoteStream.Subscribe(isin)
	}

	// Create a regular exit condition
	timeToStop := time.After(time.Minute)

	// Create a loop to process updates
	processUpdates := true

	for processUpdates {
		select {
		case err := <-errChan:
			processUpdates = false

			// An error occured. Identify which one
			if err == lemon.ErrConnectionClosed {
				fmt.Println("Connection closed by remote peer")
			} else if err == lemon.ErrUnknownISIN {
				fmt.Println("Unknown or invalid ISIN in subscription")
				processUpdates = true // Override exit decision. It's not a critical one
			} else {
				fmt.Println("Unknown Error: ", err.Error())
			}

		case tick := <-tickChan:
			// A tick update was received: Print all available fields
			fmt.Printf("Tick: %s -> %.4f, Quantity: %d\n", tick.ISIN, tick.Price, tick.Quantity)

		case quote := <-quoteChan:
			// A quote update was received: Print all available fields
			fmt.Printf("Quote: %s -> Bid = %.4f, Ask = %.4f, Bidsize: %d, Asksize: %d\n", quote.ISIN,
				quote.Bid, quote.Ask, quote.Bidsize, quote.Asksize)

		case <-timeToStop:
			// Time to stop. Disconnect on our behalf
			fmt.Println("It's time to stop")
			processUpdates = false
			tickStream.Disconnect()
			quoteStream.Disconnect()
		}
	}

	// Disconnect from the streams (not a must and does not hurt even if already closed)
	tickStream.Disconnect()
	quoteStream.Disconnect()

	// Cleanup
	close(tickChan)
	close(quoteChan)
	close(errChan)

	for len(tickChan) > 0 {
		<-tickChan
	}

	for len(quoteChan) > 0 {
		<-quoteChan
	}

	for len(errChan) > 0 {
		<-errChan
	}

	// Done
}
```
