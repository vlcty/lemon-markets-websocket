# lemon-markets-websocket

A go wrapper lib for the [lemon.markets](https://lemon.markets) WebSocket streams.

## Installation

Install using go get:

```
go get -u github.com/vlcty/lemon-markets-websocket
```

## Capabilities

Lemon.markets provides two streams:

Ticks:   
Ticks are price updates containing the current market price for a security. It's sent when a tade occured. The quantity value tells you the amount of traded shares. If quantity is 0 then no actual trade happened, but the market maker Lang und Schwarz set a new price.

Quotes:   
Quotes contain the current bid and ask spread and its sizes.

## Connection handling

The library keeps track of the connection in the background. Automatic reconnects are done when the connection drops.

## Use of channels

This library uses channels to communicate with your application. You are responsible for these channels! Depending on the amount of subscribed securities you may want to use buffered or unbuffered channels. Make sure that you close and empty them after you disconnect from the stream.

## Example code

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
	tickStream := lemon.NewTickStream(tickChan, errChan)

	// Create a stream struct for quote updates and pass in the needed channels
	quoteStream := lemon.NewQuoteStream(quoteChan, errChan)

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

	// Create an exit condition
	timeToStop := time.After(time.Minute)

	// Create a loop to process updates
	processUpdates := true

	for processUpdates {
		select {
		case err := <-errChan:
            fmt.Printf("An error occured: %s\n", err.Error())

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
		}
	}

	// Disconnect from the streams
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
}
```
