package binance

import (
	"encoding/json"
	"strconv"

	"github.com/shopspring/decimal"
)

type ErrorInfo struct {
	Code    int32  `json:"code"`
	Message string `json:"msg"`
}

type Balance struct {
	Asset  string  `json:"asset"`
	Free   float64 `json:"free,string"`
	Locked float64 `json:"locked,string"`
}

type AccountInfo struct {
	MakerCommission  int        `json:"makerCommission"`
	TakerCommission  int        `json:"takerCommission"`
	BuyerCommission  int        `json:"buyerCommission"`
	SellerCommission int        `json:"sellerCommission"`
	CanTrade         bool       `json:"canTrade"`
	CanWithdraw      bool       `json:"canWithdraw"`
	CanDeposit       bool       `json:"canDeposit"`
	Balances         []*Balance `json:"balances"`
}

type OrderType string

const (
	OrderTypeMarket          OrderType = "MARKET"
	OrderTypeLimit           OrderType = "LIMIT"
	OrderTypeStopLoss        OrderType = "STOP_LOSS"
	OrderTypeStopLossLimit   OrderType = "STOP_LOSS_LIMIT"
	OrderTypeTakeProfit      OrderType = "TAKE_PROFIT"
	OrderTypeTakeProfitLimit OrderType = "TAKE_PROFIT_LIMIT"
	OrderTypeLimitMaker      OrderType = "LIMIT_MAKER"
)

type OrderStatus string

const (
	OrderStatusNew           OrderStatus = "NEW"
	OrderStatusPartial       OrderStatus = "PARTIALLY_FILLED"
	OrderStatusFilled        OrderStatus = "FILLED"
	OrderStatusCanceled      OrderStatus = "CANCELED"
	OrderStatusPendingCancel OrderStatus = "PENDING_CANCEL" // currently unused
	OrderStatusRejected      OrderStatus = "REJECTED"
	OrderStatusExpired       OrderStatus = "EXPIRED"
)

type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "GTC" // Good Till Cancel
	TimeInForceIOC TimeInForce = "IOC" // Immediate or Cancel
	TimeInForceFOK TimeInForce = "FOK" // Fill or Kill
)

type Order struct {
	Symbol        string      `json:"symbol"`
	OrderID       int64       `json:"orderId"`
	ClientOrderID string      `json:"clientOrderId"`
	Price         float64     `json:"price,string"`
	OrigQty       float64     `json:"origQty,string"`
	ExecutedQty   float64     `json:"executedQty,string"`
	Status        OrderStatus `json:"status"`
	TimeInForce   TimeInForce `json:"timeInForce"`
	Type          OrderType   `json:"type"`
	Side          OrderSide   `json:"side"`
	StopPrice     float64     `json:"stopPrice,string"`
	IcebergQty    float64     `json:"IcebergQty,string"`
	Time          int64       `json:"time"`
	IsWorking     bool        `json:"isWorking"`
}

type ExchangeInfo struct {
	Symbols []SymbolInfo
}

type SymbolStatus string

const (
	SymbolStatusTrading SymbolStatus = "TRADING"
)

type FilterType string

const (
	FilterTypePrice       FilterType = "PRICE_FILTER"
	FilterTypeLotSize     FilterType = "LOT_SIZE"
	FilterTypeMinNotional FilterType = "MIN_NOTIONAL"
)

type SymbolInfoFilter struct {
	Type FilterType `json:"filterType"`

	// PRICE_FILTER parameters
	MinPrice decimal.Decimal `json:"minPrice,string"`
	MaxPrice decimal.Decimal `json:"maxPrice,string"`
	TickSize decimal.Decimal `json:"tickSize,string"`

	// LOT_SIZE parameters
	MinQty   decimal.Decimal `json:"minQty,string"`
	MaxQty   decimal.Decimal `json:"maxQty,string"`
	StepSize decimal.Decimal `json:"stepSize,string"`

	// MIN_NOTIONAL parameters
	MinNotional decimal.Decimal `json:"minNotional,string"`
}

type SymbolInfo struct {
	Symbol              string             `json:"symbol"`
	Status              SymbolStatus       `json:"status"`
	BaseAsset           string             `json:"baseAsset"`
	BaseAssetPrecision  int                `json:"baseAssetPrecision"`
	QuoteAsset          string             `json:"quoteAsset"`
	QuoteAssetPrecision int                `json:"quoteAssetPrecision"`
	OrderTypes          []OrderType        `json:"orderTypes"`
	Iceberg             bool               `json:"icebergAllowed"`
	Filters             []SymbolInfoFilter `json:"filters"`
}

type PostOrderAckResponse struct {
	Symbol        string `json:"symbol"`
	OrderID       int64  `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	TransactTime  int64  `json:"transactTime"`
}

type DeleteOrderResponse struct {
	Symbol            string `json:"symbol"`
	OrigClientOrderID string `json:"origClientOrderId"`
	OrderID           int64  `json:"orderId"`
	ClientOrderID     string `json:"clientOrderId"`
}

type OrderbookEntry struct {
	Price    float64 `json:",string"`
	Quantity float64 `json:",string"`
}

// UnmarshalJSON does some custom unmarshalling of orderbook entries.
func (entry *OrderbookEntry) UnmarshalJSON(b []byte) error {
	var s [2]string

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	entry.Price, err = strconv.ParseFloat(s[0], 64)
	if err != nil {
		return err
	}

	entry.Quantity, err = strconv.ParseFloat(s[1], 64)
	if err != nil {
		return err
	}

	return nil
}

type MarketData struct {
	LastUpdateID int64            `json:"lastUpdateId"`
	Bids         []OrderbookEntry `json:"bids"`
	Asks         []OrderbookEntry `json:"asks"`
}
