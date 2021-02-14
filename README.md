# lemon-markets-websocket

A go wrapper lib for the [lemon.markets](https://lemon.markets) WebSocket streams.

## Example code

```go
package main

import (
	"fmt"
	"github.com/vlcty/lemon-markets-websocket"
)

func main() {
	stream, streamError := lemon.NewLemonMarketsStream()

	if streamError != nil {
		fmt.Println("Faled to connect to lemon.markets")
	} else {
		stream.Subscribe("DE000TUAG000") // TUI AG
		stream.Subscribe("US88160R1014") // Tesla
		stream.Subscribe("LU0274208692") // Xtrackers MSCI World

		processData := true

		for processData {
			select {
			case error := <-stream.ErrorPipe():
				fmt.Println(error)
				processData = false

			case priceUpdate := <-stream.MarketPricePipe():
				fmt.Printf("ISIN: %s -> New market price: %.4f\n", priceUpdate.ISIN, priceUpdate.Price)

			case quoteUpdate := <-stream.QuotePipe():
				fmt.Printf("ISIN: %s -> New Quote: Bid=%.4f Ask=%.4f Bidsize=%.4f Asksize=%.4f\n", quoteUpdate.ISIN,
					quoteUpdate.Bid, quoteUpdate.Ask, quoteUpdate.Bidsize, quoteUpdate.Asksize)
			}
		}

		// Always call disconnect to ensure everything is disassembled properly
		stream.Disconnect()
	}
}
```

As you can see you receive your information over three channels:

- `stream.ErrorPipe()` which returns `error`
- `stream.MarketPricePipe()` which returns `LemonMarketsPriceUpdate`
- `stream.QuotePipe()` which returns `LemonMarketsQuoteUpdate`

Type definitions:

```go
type LemonMarketsPriceUpdate struct {
	ISIN     string
	Price    float64
	Quantity float64
}

type LemonMarketsQuoteUpdate struct {
	ISIN string
	Bid  float64
	Ask  float64
	Bidsize float64
	Asksize float64
}
```

If you are developing while Land und Schwarz is closed you can fake inputs via the functions `GeneratePriceUpdate` and `GenerateQuoteUpdate`. See source code for more info.
