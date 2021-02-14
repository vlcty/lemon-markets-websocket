package lemon

import (
	"errors"
	"github.com/gorilla/websocket"
)

var (
	ErrConnectFailed    error = errors.New("Can't connect to lemon markets")
	ErrConnectionClosed error = errors.New("Lemon markets connection closed")
)

const (
	ACTION_SUBSCRIBE                       string = "subscribe"
	ACTION_UNSUBSCRIBE                     string = "subscribe"
	SPECIFIER_WITH_QUANTITY                string = "with-quantity"
	SPECIFIER_WITH_UNCOVERED               string = "with-uncovered"
	SPECIFIER_WITH_QUANTITIY_AND_UNCOVERED string = "with-quantity-with-uncovered"
	SPECIFIER_WITH_QUANTITY_WITH_PRICE     string = "with-quantity-with-price"
)

type lemonMarketSubscription struct {
	Action    string `json:"action"`
	Specifier string `json:"specifier"`
	ISIN      string `json:"value"`
}

type LemonMarketsPriceUpdate struct {
	ISIN     string  `json:"isin"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

type LemonMarketsQuoteUpdate struct {
	ISIN    string  `json:"isin"`
	Bid     float64 `json:"bid_price"`
	Ask     float64 `json:"ask_price"`
	Bidsize float64 `json:"bid_size"`
	Asksize float64 `json:"ask_size"`
}

func (lmpr LemonMarketsPriceUpdate) WasTrade() bool {
	return lmpr.Quantity > 0
}

type LemonMarketsStream struct {
	marketdataConnection *websocket.Conn
	quotesConnection     *websocket.Conn
	quitWorkers          bool
	subscriptions        map[string]int
	marketpricepipe      chan LemonMarketsPriceUpdate
	quotepipe            chan LemonMarketsQuoteUpdate
	errorpipe            chan error
}

func (lms *LemonMarketsStream) Subscribe(isin string) {
	if _, exists := lms.subscriptions[isin]; !exists {
		lms.subscriptions[isin] = 1
		lms.marketdataConnection.WriteJSON(lemonMarketSubscription{
			Action:    ACTION_SUBSCRIBE,
			Specifier: SPECIFIER_WITH_QUANTITIY_AND_UNCOVERED,
			ISIN:      isin})

		lms.quotesConnection.WriteJSON(lemonMarketSubscription{
			Action:    ACTION_SUBSCRIBE,
			Specifier: SPECIFIER_WITH_QUANTITY_WITH_PRICE,
			ISIN:      isin})
	}
}

func (lms *LemonMarketsStream) Unsubscribe(isin string) {
	if _, exists := lms.subscriptions[isin]; exists {
		delete(lms.subscriptions, isin)

		lms.marketdataConnection.WriteJSON(lemonMarketSubscription{
			Action: ACTION_UNSUBSCRIBE,
			ISIN:   isin})

		lms.quotesConnection.WriteJSON(lemonMarketSubscription{
			Action: ACTION_UNSUBSCRIBE,
			ISIN:   isin})
	}
}

func (lms *LemonMarketsStream) tradeWorker() {

	for lms.quitWorkers {
		answer := LemonMarketsPriceUpdate{}
		err := lms.marketdataConnection.ReadJSON(&answer)

		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				lms.errorpipe <- ErrConnectFailed
			} else {
				lms.errorpipe <- err
			}

			lms.quitWorkers = true
		} else if len(answer.ISIN) > 0 {
			lms.marketpricepipe <- answer
		}
	}
}

func (lms *LemonMarketsStream) quotesWorker() {

	for lms.quitWorkers {
		answer := LemonMarketsQuoteUpdate{}
		err := lms.quotesConnection.ReadJSON(&answer)

		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				lms.errorpipe <- ErrConnectFailed
			} else {
				lms.errorpipe <- err
			}

			lms.quitWorkers = true
		} else if len(answer.ISIN) > 0 {
			lms.quotepipe <- answer
		}
	}
}

func (lms *LemonMarketsStream) MarketPricePipe() chan LemonMarketsPriceUpdate {
	return lms.marketpricepipe
}

func (lms *LemonMarketsStream) QuotePipe() chan LemonMarketsQuoteUpdate {
	return lms.quotepipe
}

func (lms *LemonMarketsStream) ErrorPipe() chan error {
	return lms.errorpipe
}

func NewLemonMarketsStream() (*LemonMarketsStream, error) {
	lms := &LemonMarketsStream{}

	if err := lms.connect(); err != nil {
		return nil, err
	} else {
		return lms, nil
	}
}

func (lms *LemonMarketsStream) connect() error {
	marketdataConnection, _, marketdataConnectionErr := websocket.DefaultDialer.Dial("wss://api.lemon.markets/streams/v1/marketdata", nil)
	quotesConnection, _, quotesConnectionErr := websocket.DefaultDialer.Dial("wss://api.lemon.markets/streams/v1/quotes", nil)

	if marketdataConnectionErr != nil {
		return ErrConnectFailed
	} else if quotesConnectionErr != nil {
		return ErrConnectFailed
	} else {
		lms.marketdataConnection = marketdataConnection
		lms.quotesConnection = quotesConnection
		lms.quitWorkers = false
		lms.subscriptions = make(map[string]int)
		lms.marketpricepipe = make(chan LemonMarketsPriceUpdate, 20)
		lms.quotepipe = make(chan LemonMarketsQuoteUpdate, 20)
		lms.errorpipe = make(chan error, 5)

		go lms.tradeWorker()
		go lms.quotesWorker()

		return nil
	}
}

func (lms *LemonMarketsStream) Disconnect() {
	lms.quitWorkers = true

	close(lms.marketpricepipe)

	for len(lms.marketpricepipe) > 0 {
		<-lms.marketpricepipe
	}

	close(lms.quotepipe)

	for len(lms.quotepipe) > 0 {
		<-lms.quotepipe
	}

	close(lms.errorpipe)

	for len(lms.errorpipe) > 0 {
		<-lms.errorpipe
	}
}

func (lms *LemonMarketsStream) GeneratePriceUpdate(isin string, marketPrice, quantity float64) {
	lms.marketpricepipe <- LemonMarketsPriceUpdate{ISIN: isin, Price: marketPrice, Quantity: quantity}
}

func (lms *LemonMarketsStream) GenerateQuoteUpdate(isin string, bid, ask, bidsize, asksize float64) {
	lms.quotepipe <- LemonMarketsQuoteUpdate{ISIN: isin, Bid: bid, Ask: ask, Bidsize: bidsize, Asksize: asksize}
}
