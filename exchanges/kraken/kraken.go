package kraken

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/config"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
	"github.com/shopspring/decimal"
)

const (
	KRAKEN_API_URL        = "https://api.kraken.com"
	KRAKEN_API_VERSION    = "0"
	KRAKEN_SERVER_TIME    = "Time"
	KRAKEN_ASSETS         = "Assets"
	KRAKEN_ASSET_PAIRS    = "AssetPairs"
	KRAKEN_TICKER         = "Ticker"
	KRAKEN_OHLC           = "OHLC"
	KRAKEN_DEPTH          = "Depth"
	KRAKEN_TRADES         = "Trades"
	KRAKEN_SPREAD         = "Spread"
	KRAKEN_BALANCE        = "Balance"
	KRAKEN_TRADE_BALANCE  = "TradeBalance"
	KRAKEN_OPEN_ORDERS    = "OpenOrders"
	KRAKEN_CLOSED_ORDERS  = "ClosedOrders"
	KRAKEN_QUERY_ORDERS   = "QueryOrders"
	KRAKEN_TRADES_HISTORY = "TradesHistory"
	KRAKEN_QUERY_TRADES   = "QueryTrades"
	KRAKEN_OPEN_POSITIONS = "OpenPositions"
	KRAKEN_LEDGERS        = "Ledgers"
	KRAKEN_QUERY_LEDGERS  = "QueryLedgers"
	KRAKEN_TRADE_VOLUME   = "TradeVolume"
	KRAKEN_ORDER_CANCEL   = "CancelOrder"
	KRAKEN_ORDER_PLACE    = "AddOrder"
)

const (
	OrderStatusPending   = "pending"
	OrderStatusOpen      = "open"
	OrderStatusClosed    = "closed"
	OrderStatusCancelled = "canceled"
	OrderStatusExpired   = "expired"
)

const (
	OrderTypeLimit = "limit"
)

type Kraken struct {
	exchange.Base
	CryptoFee, FiatFee float64
	Ticker             map[string]KrakenTicker
	// Maps a currency pair of the form XXX/YYY to a symbol (exchange specific market identifier)
	CurrencyPairCodeToSymbol map[pair.CurrencyItem]string
	// Maps symbol (exchange specific market identifier) to currency pair info
	CurrencyPairs map[pair.CurrencyItem]*exchange.CurrencyPairInfo
	// Maps a currency pair of the form XXX/YYY to max num of decimal places
	// Kraken allows to be specified for the price of orders placed for the currency pair.
	PriceDecimalPlaces map[pair.CurrencyItem]int32
}

func (k *Kraken) SetDefaults() {
	k.Name = "Kraken"
	k.Enabled = false
	k.FiatFee = 0.35
	k.CryptoFee = 0.10
	k.Verbose = false
	k.Websocket = false
	k.RESTPollingDelay = 10
	k.Ticker = make(map[string]KrakenTicker)
	k.RequestCurrencyPairFormat.Delimiter = ""
	k.RequestCurrencyPairFormat.Uppercase = true
	k.RequestCurrencyPairFormat.Separator = ","
	k.ConfigCurrencyPairFormat.Delimiter = ""
	k.ConfigCurrencyPairFormat.Uppercase = true
	k.AssetTypes = []string{ticker.Spot}
	k.Orderbooks = orderbook.Init()
}

func (k *Kraken) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		k.SetEnabled(false)
	} else {
		k.Enabled = true
		k.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		k.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		k.RESTPollingDelay = exch.RESTPollingDelay
		k.Verbose = exch.Verbose
		k.Websocket = exch.Websocket
		k.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
		k.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
		k.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")
		err := k.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = k.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

// CurrencyPairToSymbol converts a currency pair to a symbol (exchange specific market identifier).
func (k *Kraken) CurrencyPairToSymbol(p pair.CurrencyPair) (string, error) {
	currencyPairCode := p.Display("/", true)
	if symbol, exists := k.CurrencyPairCodeToSymbol[currencyPairCode]; exists {
		return symbol, nil
	}
	return "", fmt.Errorf("failed to map currency pair '%s' to a Kraken asset pair", currencyPairCode)
}

// SymbolToCurrencyPair converts a symbol (exchange specific market identifier) to a currency pair.
func (k *Kraken) SymbolToCurrencyPair(symbol string) (pair.CurrencyPair, error) {
	if info, exists := k.CurrencyPairs[pair.CurrencyItem(symbol)]; exists {
		return info.Currency.FormatPair(
			k.RequestCurrencyPairFormat.Delimiter,
			k.RequestCurrencyPairFormat.Uppercase,
		), nil
	}
	return pair.CurrencyPair{},
		fmt.Errorf("failed to map Kraken asset pair '%s' to a currency pair", symbol)
}

type currencyLimits struct {
	exchangeName       string
	priceDecimalPlaces map[pair.CurrencyItem]int32
}

// Source: https://support.kraken.com/hc/en-us/articles/205893708-What-is-the-minimum-order-size-
var minTradeSizes = map[pair.CurrencyItem]float64{
	"REP":  0.3,
	"XBT":  0.002,
	"BCH":  0.002,
	"DASH": 0.03,
	"DOGE": 3000,
	"EOS":  3,
	"ETH":  0.02,
	"ETC":  0.3,
	"GNO":  0.03,
	"ICN":  2,
	"LTC":  0.1,
	"MLN":  0.1,
	"XMR":  0.1,
	"XRP":  30,
	"XLM":  300,
	"ZEC":  0.03,
	"USDT": 5,
}

func newCurrencyLimits(exchangeName string, priceDecimalPlaces map[pair.CurrencyItem]int32) *currencyLimits {
	return &currencyLimits{exchangeName, priceDecimalPlaces}
}

// Returns max number of decimal places allowed in the trade price for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (cl *currencyLimits) GetPriceDecimalPlaces(p pair.CurrencyPair) int32 {
	k := p.Display("/", true)
	if v, exists := cl.priceDecimalPlaces[k]; exists {
		return v
	}
	return -1
}

// Returns max number of decimal places allowed in the trade amount for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (cl *currencyLimits) GetAmountDecimalPlaces(p pair.CurrencyPair) int32 {
	// API docs don't mention anything about this so make an educated guess...
	return 8
}

// Returns the minimum trade amount for the given currency pair.
func (cl *currencyLimits) GetMinAmount(p pair.CurrencyPair) float64 {
	k := p.FirstCurrency.Upper()
	if v, exists := minTradeSizes[k]; exists {
		return v
	}
	return 0
}

// GetLimits returns price/amount limits for the exchange.
func (k *Kraken) GetLimits() exchange.ILimits {
	return newCurrencyLimits(k.Name, k.PriceDecimalPlaces)
}

// Returns currency pairs that can be used by the exchange account associated with this bot.
// Use FormatExchangeCurrency to get the right key.
func (k *Kraken) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	return k.CurrencyPairs
}

// GetOrder returns information about the exchange order matching the given ID
func (k *Kraken) GetOrder(orderID string, currencyPair pair.CurrencyPair) (*exchange.Order, error) {
	panic("not implemented")
}

func (k *Kraken) GetOrders(pairs []pair.CurrencyPair) ([]*exchange.Order, error) {
	orders, err := k.GetOpenOrders(false, 0)
	if err != nil {
		return nil, err
	}

	ret := []*exchange.Order{}
	for orderID, order := range orders {
		// TODO: filter out orders that don't match the given pairs
		exchangeOrder, err := k.convertOrderToExchangeOrder(orderID, &order)
		if err != nil {
			log.Print(err)
		} else {
			ret = append(ret, exchangeOrder)
		}
	}
	return ret, nil
}

func (k *Kraken) convertOrderToExchangeOrder(orderID string, order *Order) (*exchange.Order, error) {
	retOrder := &exchange.Order{}
	retOrder.OrderID = orderID

	switch order.Status {
	case OrderStatusPending, OrderStatusOpen:
		retOrder.Status = exchange.OrderStatusActive
	case OrderStatusCancelled, OrderStatusExpired:
		retOrder.Status = exchange.OrderStatusAborted
	case OrderStatusClosed:
		retOrder.Status = exchange.OrderStatusFilled
	default:
		return nil, fmt.Errorf("unsupported order with status '%s'", order.Status)
	}

	retOrder.Amount = order.Volume
	retOrder.FilledAmount = order.VolumeExecuted
	retOrder.RemainingAmount, _ = decimal.NewFromFloat(order.Volume).
		Sub(decimal.NewFromFloat(order.VolumeExecuted)).Float64()

	if retOrder.Status == exchange.OrderStatusActive {
		retOrder.Rate = order.Info.Price
	} else if order.AvgPrice == 0 {
		retOrder.Rate = order.Info.Price
	} else {
		retOrder.Rate = order.AvgPrice
	}

	var createdAt int64
	// Drop the fractional part of the timestamp, whatever it is.
	timeParts := strings.Split(order.OpenTimestamp, ".")
	if len(timeParts) > 0 {
		createdAt, _ = strconv.ParseInt(timeParts[0], 10, 64)
	}
	retOrder.CreatedAt = createdAt

	retOrder.CurrencyPair, _ = k.SymbolToCurrencyPair(order.Info.Pair)
	retOrder.Side = exchange.OrderSide(order.Info.Side)
	if order.Info.Type == OrderTypeLimit {
		retOrder.Type = exchange.OrderTypeExchangeLimit
	} else {
		return nil, fmt.Errorf("unsupported order with type '%s'", order.Info.Type)
	}

	return retOrder, nil
}

// NewOrder submits a new order and returns the ID of the new exchange order
func (k *Kraken) NewOrder(currencyPair pair.CurrencyPair, amount, price float64,
	side exchange.OrderSide, orderType exchange.OrderType) (string, error) {
	symbol, err := k.CurrencyPairToSymbol(currencyPair)
	if err != nil {
		return "", err
	}
	result, err := k.AddOrder(AddOrderParams{
		Pair:         symbol,
		Side:         side,
		Type:         orderType,
		Price:        price,
		Volume:       amount,
		UserRef:      0,
		OnlyValidate: true,
	})
	if err != nil {
		return "", err
	}
	// TODO: figure out where the freaking order ID is
	return result.TransactionIDs[0], nil
}

func (k *Kraken) GetFee(cryptoTrade bool) float64 {
	if cryptoTrade {
		return k.CryptoFee
	} else {
		return k.FiatFee
	}
}

func (k *Kraken) GetServerTime() error {
	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_SERVER_TIME)
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return err
	}

	log.Println(result)
	return nil
}

func (k *Kraken) GetAssets() (map[string]KrakenAsset, error) {
	var result map[string]KrakenAsset
	path := fmt.Sprintf("%s/%s/public/%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_ASSETS)
	err := k.HTTPRequest(path, false, url.Values{}, &result)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (k *Kraken) GetAssetPairs() (map[string]KrakenAssetPairs, error) {
	var result map[string]KrakenAssetPairs
	path := fmt.Sprintf("%s/%s/public/%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_ASSET_PAIRS)
	err := k.HTTPRequest(path, false, url.Values{}, &result)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (k *Kraken) GetTicker(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	type Response struct {
		Error []interface{}                   `json:"error"`
		Data  map[string]KrakenTickerResponse `json:"result"`
	}

	resp := Response{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_TICKER, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &resp)

	if err != nil {
		return err
	}

	if len(resp.Error) > 0 {
		return errors.New(fmt.Sprintf("Kraken error: %s", resp.Error))
	}

	for x, y := range resp.Data {
		x = x[1:4] + x[5:]
		ticker := KrakenTicker{}
		ticker.Ask, _ = strconv.ParseFloat(y.Ask[0], 64)
		ticker.Bid, _ = strconv.ParseFloat(y.Bid[0], 64)
		ticker.Last, _ = strconv.ParseFloat(y.Last[0], 64)
		ticker.Volume, _ = strconv.ParseFloat(y.Volume[1], 64)
		ticker.VWAP, _ = strconv.ParseFloat(y.VWAP[1], 64)
		ticker.Trades = y.Trades[1]
		ticker.Low, _ = strconv.ParseFloat(y.Low[1], 64)
		ticker.High, _ = strconv.ParseFloat(y.High[1], 64)
		ticker.Open, _ = strconv.ParseFloat(y.Open, 64)
		k.Ticker[x] = ticker
	}
	return nil
}

func (k *Kraken) GetOHLC(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_OHLC, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return err
	}

	log.Println(result)
	return nil
}

// GetDepth returns the orderbook for a particular currency
func (k *Kraken) GetDepth(symbol string) (Orderbook, error) {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	var ob Orderbook
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_DEPTH, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return ob, err
	}

	data := result.(map[string]interface{})
	orderbookData := data["result"].(map[string]interface{})

	var bidsData []interface{}
	var asksData []interface{}
	for _, y := range orderbookData {
		yData := y.(map[string]interface{})
		bidsData = yData["bids"].([]interface{})
		asksData = yData["asks"].([]interface{})
	}

	processOrderbook := func(data []interface{}) ([]OrderbookBase, error) {
		var result []OrderbookBase
		for x := range data {
			entry := data[x].([]interface{})

			price, err := strconv.ParseFloat(entry[0].(string), 64)
			if err != nil {
				return nil, err
			}

			amount, err := strconv.ParseFloat(entry[1].(string), 64)
			if err != nil {
				return nil, err
			}

			result = append(result, OrderbookBase{Price: price, Amount: amount})
		}
		return result, nil
	}

	ob.Bids, err = processOrderbook(bidsData)
	if err != nil {
		return ob, err
	}

	ob.Asks, err = processOrderbook(asksData)
	if err != nil {
		return ob, err
	}

	return ob, nil
}

func (k *Kraken) GetTrades(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_TRADES, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		return err
	}

	log.Println(result)
	return nil
}

func (k *Kraken) GetSpread(symbol string) {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	path := fmt.Sprintf("%s/%s/public/%s?%s", KRAKEN_API_URL, KRAKEN_API_VERSION, KRAKEN_SPREAD, values.Encode())
	err := common.SendHTTPGetRequest(path, true, k.Verbose, &result)

	if err != nil {
		log.Println(err)
		return
	}
}

func (k *Kraken) GetBalance() error {
	var result interface{}
	err := k.HTTPRequest(KRAKEN_BALANCE, true, url.Values{}, &result)
	if err != nil {
		return err
	}
	panic("not implemented")
}

func (k *Kraken) GetTradeBalance(symbol, asset string) error {
	values := url.Values{}

	if len(symbol) > 0 {
		values.Set("aclass", symbol)
	}

	if len(asset) > 0 {
		values.Set("asset", asset)
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_TRADE_BALANCE, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetOpenOrders(showTrades bool, userref int64) (map[string]Order, error) {
	values := url.Values{}

	if showTrades {
		values.Set("trades", "true")
	}

	if userref != 0 {
		values.Set("userref", strconv.FormatInt(userref, 10))
	}

	type OpenOrdersResponse struct {
		Open map[string]Order `json:"open"`
	}
	var result OpenOrdersResponse
	err := k.HTTPRequest(KRAKEN_OPEN_ORDERS, true, values, &result)

	if err != nil {
		return nil, err
	}
	return result.Open, nil
}

func (k *Kraken) GetClosedOrders(showTrades bool, userref, start, end, offset int64, closetime string) error {
	values := url.Values{}

	if showTrades {
		values.Set("trades", "true")
	}

	if userref != 0 {
		values.Set("userref", strconv.FormatInt(userref, 10))
	}

	if start != 0 {
		values.Set("start", strconv.FormatInt(start, 10))
	}

	if end != 0 {
		values.Set("end", strconv.FormatInt(end, 10))
	}

	if offset != 0 {
		values.Set("ofs", strconv.FormatInt(offset, 10))
	}

	if len(closetime) > 0 {
		values.Set("closetime", closetime)
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_CLOSED_ORDERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) QueryOrdersInfo(showTrades bool, userref, txid int64) error {
	values := url.Values{}

	if showTrades {
		values.Set("trades", "true")
	}

	if userref != 0 {
		values.Set("userref", strconv.FormatInt(userref, 10))
	}

	if txid != 0 {
		values.Set("txid", strconv.FormatInt(userref, 10))
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_QUERY_ORDERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetTradesHistory(tradeType string, showRelatedTrades bool, start, end, offset int64) error {
	values := url.Values{}

	if len(tradeType) > 0 {
		values.Set("aclass", tradeType)
	}

	if showRelatedTrades {
		values.Set("trades", "true")
	}

	if start != 0 {
		values.Set("start", strconv.FormatInt(start, 10))
	}

	if end != 0 {
		values.Set("end", strconv.FormatInt(end, 10))
	}

	if offset != 0 {
		values.Set("offset", strconv.FormatInt(offset, 10))
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_TRADES_HISTORY, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) QueryTrades(txid int64, showRelatedTrades bool) error {
	values := url.Values{}
	values.Set("txid", strconv.FormatInt(txid, 10))

	if showRelatedTrades {
		values.Set("trades", "true")
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_QUERY_TRADES, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) OpenPositions(txid int64, showPL bool) error {
	values := url.Values{}
	values.Set("txid", strconv.FormatInt(txid, 10))

	if showPL {
		values.Set("docalcs", "true")
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_OPEN_POSITIONS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetLedgers(symbol, asset, ledgerType string, start, end, offset int64) error {
	values := url.Values{}

	if len(symbol) > 0 {
		values.Set("aclass", symbol)
	}

	if len(asset) > 0 {
		values.Set("asset", asset)
	}

	if len(ledgerType) > 0 {
		values.Set("type", ledgerType)
	}

	if start != 0 {
		values.Set("start", strconv.FormatInt(start, 10))
	}

	if end != 0 {
		values.Set("end", strconv.FormatInt(end, 10))
	}

	if offset != 0 {
		values.Set("offset", strconv.FormatInt(offset, 10))
	}

	var result interface{}
	err := k.HTTPRequest(KRAKEN_LEDGERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) QueryLedgers(id string) error {
	values := url.Values{}
	values.Set("id", id)

	var result interface{}
	err := k.HTTPRequest(KRAKEN_QUERY_LEDGERS, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) GetTradeVolume(symbol string) error {
	values := url.Values{}
	values.Set("pair", symbol)

	var result interface{}
	err := k.HTTPRequest(KRAKEN_TRADE_VOLUME, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

type AddOrderParams struct {
	Pair         string
	Side         exchange.OrderSide
	Type         exchange.OrderType
	Price        float64
	Volume       float64
	UserRef      int32
	OnlyValidate bool
}

func (k *Kraken) AddOrder(params AddOrderParams) (*AddOrderResult, error) {
	values := url.Values{}
	values.Set("pair", params.Pair)
	values.Set("type", string(params.Side))

	if params.Type == exchange.OrderTypeExchangeLimit {
		values.Set("ordertype", "limit")
	} else {
		return nil, fmt.Errorf("support for '%s' orders hasn't been implemented")
	}

	values.Set("price", strconv.FormatFloat(params.Price, 'f', -1, 64))
	values.Set("volume", strconv.FormatFloat(params.Volume, 'f', -1, 64))
	if params.OnlyValidate {
		values.Set("validate", "true")
	}

	var result AddOrderResult
	err := k.HTTPRequest(KRAKEN_ORDER_PLACE, true, values, &result)

	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (k *Kraken) CancelOrder(orderStr string, currencyPair pair.CurrencyPair) error {
	var orderID int64
	var err error
	if orderID, err = strconv.ParseInt(orderStr, 10, 64); err != nil {
		return err
	}
	return k.cancelOrder(orderID)
}

func (k *Kraken) cancelOrder(orderID int64) error {
	values := url.Values{}
	values.Set("txid", strconv.FormatInt(orderID, 10))

	var result interface{}
	err := k.HTTPRequest(KRAKEN_ORDER_CANCEL, true, values, &result)

	if err != nil {
		return err
	}

	panic("not implemented")
}

func (k *Kraken) SendAuthenticatedHTTPRequest(method string, values url.Values, result interface{}) error {
	if !k.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, k.Name)
	}

	path := fmt.Sprintf("/%s/private/%s", KRAKEN_API_VERSION, method)
	if k.Nonce.Get() == 0 {
		k.Nonce.Set(time.Now().UnixNano())
	} else {
		k.Nonce.Inc()
	}

	values.Set("nonce", k.Nonce.String())
	secret, err := common.Base64Decode(k.APISecret)

	if err != nil {
		return err
	}

	shasum := common.GetSHA256([]byte(values.Get("nonce") + values.Encode()))
	signature := common.Base64Encode(common.GetHMAC(common.HashSHA512, append([]byte(path), shasum...), secret))

	if k.Verbose {
		log.Printf("Sending POST request to %s, path: %s.", KRAKEN_API_URL, path)
	}

	headers := make(map[string]string)
	headers["API-Key"] = k.APIKey
	headers["API-Sign"] = signature

	resp, err := common.SendHTTPRequest("POST", KRAKEN_API_URL+path, headers, strings.NewReader(values.Encode()))

	if err != nil {
		return err
	}

	if k.Verbose {
		log.Printf("Received raw: \n%s\n", resp)
	}

	err = common.JSONDecode([]byte(resp), &result)
	if err != nil {
		return errors.New("Unable to JSON Unmarshal response." + err.Error())
	}

	return nil
}

// HTTPRequestJSON sends an HTTP request to a Kraken API endpoint and and returns the result as raw JSON.
func (k *Kraken) HTTPRequestJSON(path string, auth bool, values url.Values) (json.RawMessage, error) {
	response := Response{}
	if auth {
		if err := k.SendAuthenticatedHTTPRequest(path, values, &response); err != nil {
			return nil, err
		}
	} else {
		if err := common.SendHTTPGetRequest(path, true, k.Verbose, &response); err != nil {
			return nil, err
		}
	}
	if len(response.Errors) > 0 {
		return response.Result, errors.New(strings.Join(response.Errors, "\n"))
	}
	return response.Result, nil
}

// HTTPRequest is a generalized http request function.
func (k *Kraken) HTTPRequest(path string, auth bool, values url.Values, v interface{}) error {
	result, err := k.HTTPRequestJSON(path, auth, values)
	if err != nil {
		return err
	}
	return json.Unmarshal(result, &v)
}
