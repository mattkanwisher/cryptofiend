package kraken

import "encoding/json"

// Response is the generalised response type for Kraken
type Response struct {
	Errors []string        `json:"errors"`
	Result json.RawMessage `json:"result"`
}

type KrakenAsset struct {
	AltName         string `json:"altname"`
	Decimals        int    `json:"decimals"`
	DisplayDecimals int    `json:"display_decimals"`
}

type KrakenAssetPairs struct {
	Altname           string      `json:"altname"`
	AclassBase        string      `json:"aclass_base"`
	Base              string      `json:"base"`
	AclassQuote       string      `json:"aclass_quote"`
	Quote             string      `json:"quote"`
	Lot               string      `json:"lot"`
	PairDecimals      int         `json:"pair_decimals"`
	LotDecimals       int         `json:"lot_decimals"`
	LotMultiplier     int         `json:"lot_multiplier"`
	LeverageBuy       []int       `json:"leverage_buy"`
	LeverageSell      []int       `json:"leverage_sell"`
	Fees              [][]float64 `json:"fees"`
	FeesMaker         [][]float64 `json:"fees_maker"`
	FeeVolumeCurrency string      `json:"fee_volume_currency"`
	MarginCall        int         `json:"margin_call"`
	MarginStop        int         `json:"margin_stop"`
}

type KrakenTicker struct {
	Ask    float64
	Bid    float64
	Last   float64
	Volume float64
	VWAP   float64
	Trades int64
	Low    float64
	High   float64
	Open   float64
}

// OrderbookBase stores the orderbook price and amount data
type OrderbookBase struct {
	Price  float64
	Amount float64
}

// Orderbook stores the bids and asks orderbook data
type Orderbook struct {
	Bids []OrderbookBase
	Asks []OrderbookBase
}

type KrakenTickerResponse struct {
	Ask    []string `json:"a"`
	Bid    []string `json:"b"`
	Last   []string `json:"c"`
	Volume []string `json:"v"`
	VWAP   []string `json:"p"`
	Trades []int64  `json:"t"`
	Low    []string `json:"l"`
	High   []string `json:"h"`
	Open   string   `json:"o"`
}

type OrderInfo struct {
	Pair      string  `json:"pair"`
	Side      string  `json:"type"`
	Type      string  `json:"ordertype"`
	Price     float64 `json:"price,string"`
	Price2    float64 `json:"price2,string"`
	OrderDesc string  `json:"order"`
	CloseDesc string  `json:"close"`
}

type Order struct {
	RefID           string    `json:"refid"`
	UserRef         string    `json:"userref"`
	Status          string    `json:"status"`
	OpenTimestamp   string    `json:"opentm"`
	StartTimestamp  string    `json:"starttm"`
	ExpireTimestamp string    `json:"expiretm"`
	Info            OrderInfo `json:"descr"`
	Volume          float64   `json:"vol,string"`
	VolumeExecuted  float64   `json:"vol_exec,string"`
	Cost            float64   `json:"cost,string"`
	Fee             float64   `json:"fee,string"`
	AvgPrice        float64   `json:"price,string"`
	StopPrice       float64   `json:"stopprice,string"`
	LimitPrice      float64   `json:"limitprice,string"`
	Misc            string    `json:"misc"`
	Flags           string    `json:"oflags"`
	TradeIDs        []string  `json:"trades"`
}

type AddOrderResult struct {
	Info           OrderInfo `json:"descr"`
	TransactionIDs []string  `json:"txid"`
}
