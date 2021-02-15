package lemon

import (
	"errors"
	// "fmt"
	"github.com/gorilla/websocket"
)

var (
	ErrConnectFailed    error = errors.New("Can't connect to lemon markets")
	ErrConnectionClosed error = errors.New("Lemon markets connection closed")
	ErrTypeUnknown      error = errors.New("Type not known")
)

const (
	CONNECTIONTYPE_PRICE                   string = "price"
	CONNECTIONTYPE_QUOTE                   string = "quote"
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

type PriceUpdate struct {
	ISIN     string  `json:"isin"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

type QuoteUpdate struct {
	ISIN    string  `json:"isin"`
	Bid     float64 `json:"bid_price"`
	Ask     float64 `json:"ask_price"`
	Bidsize float64 `json:"bid_size"`
	Asksize float64 `json:"ask_size"`
}

func (lmpr PriceUpdate) WasTrade() bool {
	return lmpr.Quantity > 0
}

type OnPriceUpdateFunc func(PriceUpdate)
type OnQuoteUpdateFunc func(QuoteUpdate)
type OnErrorFunc func(error)

type LemonMarketsStream struct {
	connection     *websocket.Conn
	connectiontype string
	processData    bool
	subscriptions  map[string]int
	OnPriceUpdate  OnPriceUpdateFunc
	OnQuoteUpdate  OnQuoteUpdateFunc
	OnError        OnErrorFunc
}

func (lms *LemonMarketsStream) Subscribe(isin string) {
	if _, exists := lms.subscriptions[isin]; !exists {
		lms.subscriptions[isin] = 1

		switch lms.connectiontype {
		case CONNECTIONTYPE_PRICE:
			lms.connection.WriteJSON(&lemonMarketSubscription{
				Action:    ACTION_SUBSCRIBE,
				Specifier: SPECIFIER_WITH_QUANTITIY_AND_UNCOVERED,
				ISIN:      isin})

		case CONNECTIONTYPE_QUOTE:
			lms.connection.WriteJSON(&lemonMarketSubscription{
				Action:    ACTION_SUBSCRIBE,
				Specifier: SPECIFIER_WITH_QUANTITY_WITH_PRICE,
				ISIN:      isin})
		}
	}
}

func (lms *LemonMarketsStream) Unsubscribe(isin string) {
	if _, exists := lms.subscriptions[isin]; exists {
		delete(lms.subscriptions, isin)

		lms.connection.WriteJSON(&lemonMarketSubscription{
			Action: ACTION_UNSUBSCRIBE,
			ISIN:   isin})
	}
}

func Connect(connectiontype string) (*LemonMarketsStream, error) {
	lms := &LemonMarketsStream{}
	lms.connectiontype = connectiontype
	var url string

	switch connectiontype {
	case CONNECTIONTYPE_PRICE:
		url = "wss://api.lemon.markets/streams/v1/marketdata"

	case CONNECTIONTYPE_QUOTE:
		url = "wss://api.lemon.markets/streams/v1/quotes"

	default:
		return nil, ErrTypeUnknown
	}

	connection, _, connectionError := websocket.DefaultDialer.Dial(url, nil)

	if connectionError != nil {
		return nil, ErrConnectFailed
	} else {
		lms.connection = connection
		lms.processData = true
		lms.subscriptions = make(map[string]int)

		return lms, nil
	}
}

func (lms *LemonMarketsStream) handleError(err error) {
	lms.processData = false

	if lms.OnError != nil {
		lms.OnError(err)
	}

	lms.connection.Close()
}

func (lms *LemonMarketsStream) ListenToUpdates() {
	for lms.processData {
		switch lms.connectiontype {
		case CONNECTIONTYPE_PRICE:
			update := PriceUpdate{}
			if parseError := lms.connection.ReadJSON(update); parseError != nil {
				lms.handleError(parseError)
			} else if lms.OnPriceUpdate != nil {
				lms.OnPriceUpdate(update)
			}

		case CONNECTIONTYPE_QUOTE:
			update := QuoteUpdate{}
			if parseError := lms.connection.ReadJSON(update); parseError != nil {
				lms.handleError(parseError)
			} else if lms.OnQuoteUpdate != nil {
				lms.OnQuoteUpdate(update)
			}
		}
	}
}
