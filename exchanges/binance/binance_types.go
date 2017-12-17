package binance

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
	MinPrice string `json:"minPrice"`
	MaxPrice string `json:"maxPrice"`
	TickSize string `json:"tickSize"`

	// LOT_SIZE parameters
	MinQty   string `json:"minQty"`
	MaxQty   string `json:"maxQty"`
	StepSize string `json:"stepSize"`

	// MIN_NOTIONAL parameters
	MinNotional string `json:"minNotional"`
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
