package liqui

import (
	"errors"
	"fmt"
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
	log "github.com/sirupsen/logrus"
)

const (
	liquiAPIPublicURL      = "https://api.Liqui.io/api"
	liquiAPIPrivateURL     = "https://api.Liqui.io/tapi"
	liquiAPIPublicVersion  = "3"
	liquiAPIPrivateVersion = "1"
	liquiInfo              = "info"
	liquiTicker            = "ticker"
	liquiDepth             = "depth"
	liquiTrades            = "trades"
	liquiAccountInfo       = "getInfo"
	liquiTrade             = "Trade"
	liquiActiveOrders      = "ActiveOrders"
	liquiOrderInfo         = "OrderInfo"
	liquiCancelOrder       = "CancelOrder"
	liquiTradeHistory      = "TradeHistory"
	liquiWithdrawCoin      = "WithdrawCoin"
)

// Liqui is the overarching type across the liqui package
type Liqui struct {
	exchange.Base
	Info Info
}

// SetDefaults sets current default values for liqui
func (l *Liqui) SetDefaults() {
	l.Name = "Liqui"
	l.Enabled = false
	l.Fee = 0.25
	l.Verbose = false
	l.Websocket = false
	l.RESTPollingDelay = 10
	l.RequestCurrencyPairFormat.Delimiter = "_"
	l.RequestCurrencyPairFormat.Uppercase = false
	l.RequestCurrencyPairFormat.Separator = "-"
	l.ConfigCurrencyPairFormat.Delimiter = "_"
	l.ConfigCurrencyPairFormat.Uppercase = true
	l.AssetTypes = []string{ticker.Spot}
	l.Orderbooks = orderbook.Init()
}

// Setup sets exchange configuration parameters for liqui
func (l *Liqui) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		l.SetEnabled(false)
	} else {
		l.Enabled = true
		l.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		l.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		l.RESTPollingDelay = exch.RESTPollingDelay
		l.Verbose = exch.Verbose
		l.Websocket = exch.Websocket
		l.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
		l.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
		l.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")
		err := l.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = l.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

type currencyLimits struct {
	exchangeName string
	info         map[string]PairData
}

func newCurrencyLimits(exchangeName string, data map[string]PairData) *currencyLimits {
	return &currencyLimits{exchangeName, data}
}

// Returns max number of decimal places allowed in the trade price for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (l *currencyLimits) GetPriceDecimalPlaces(p pair.CurrencyPair) int32 {
	k := exchange.FormatExchangeCurrency(l.exchangeName, p).String()
	if v, exists := l.info[k]; exists {
		return int32(v.DecimalPlaces)
	}
	return -1
}

// Returns max number of decimal places allowed in the trade amount for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (l *currencyLimits) GetAmountDecimalPlaces(p pair.CurrencyPair) int32 {
	k := exchange.FormatExchangeCurrency(l.exchangeName, p).String()
	if v, exists := l.info[k]; exists {
		return int32(v.DecimalPlaces)
	}
	return -1
}

// Returns the minimum trade amount for the given currency pair.
func (l *currencyLimits) GetMinAmount(p pair.CurrencyPair) float64 {
	k := exchange.FormatExchangeCurrency(l.exchangeName, p).String()
	if v, exists := l.info[k]; exists {
		return v.MinAmount
	}
	return 0
}

// Returns the minimum trade total (amount * price) for the given currency pair.
func (l *currencyLimits) GetMinTotal(p pair.CurrencyPair) float64 {
	// Not specified by the exchange.
	return 0
}

// GetLimits returns price/amount limits for the exchange.
func (l *Liqui) GetLimits() exchange.ILimits {
	return newCurrencyLimits(l.Name, l.Info.Pairs)
}

// Returns currency pairs that can be used by the exchange account associated with this bot.
// Use FormatExchangeCurrency to get the right key.
func (l *Liqui) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	currencies := map[pair.CurrencyItem]*exchange.CurrencyPairInfo{}
	for currency, currencyInfo := range l.Info.Pairs {
		if currencyInfo.Hidden == 0 {
			p := pair.NewCurrencyPairDelimiter(currency, l.RequestCurrencyPairFormat.Delimiter)
			k := exchange.FormatExchangeCurrency(l.Name, p)
			currencies[k] = &exchange.CurrencyPairInfo{Currency: p}
		}
	}
	return currencies
}

// GetFee returns a fee for a specific currency
func (l *Liqui) GetFee(currency string) (float64, error) {
	val, ok := l.Info.Pairs[common.StringToLower(currency)]
	if !ok {
		return 0, errors.New("currency does not exist")
	}

	return val.Fee, nil
}

// GetAvailablePairs returns all available pairs
func (l *Liqui) GetAvailablePairs(nonHidden bool) []string {
	var pairs []string
	for x, y := range l.Info.Pairs {
		if nonHidden && y.Hidden == 1 || x == "" {
			continue
		}
		pairs = append(pairs, common.StringToUpper(x))
	}
	return pairs
}

// GetInfo provides all the information about currently active pairs, such as
// the maximum number of digits after the decimal point, the minimum price, the
// maximum price, the minimum transaction size, whether the pair is hidden, the
// commission for each pair.
func (l *Liqui) GetInfo() (Info, error) {
	resp := Info{}
	req := fmt.Sprintf("%s/%s/%s/", liquiAPIPublicURL, liquiAPIPublicVersion, liquiInfo)

	return resp, common.SendHTTPGetRequest(req, true, l.Verbose, &resp)
}

// GetTicker returns information about currently active pairs, such as: the
// maximum price, the minimum price, average price, trade volume, trade volume
// in currency, the last trade, Buy and Sell price. All information is provided
// over the past 24 hours.
//
// currencyPair - example "eth_btc"
func (l *Liqui) GetTicker(currencyPair string) (map[string]Ticker, error) {
	type Response struct {
		Data map[string]Ticker
	}

	response := Response{}
	req := fmt.Sprintf("%s/%s/%s/%s", liquiAPIPublicURL, liquiAPIPublicVersion, liquiTicker, currencyPair)

	return response.Data,
		common.SendHTTPGetRequest(req, true, l.Verbose, &response.Data)
}

// GetDepth information about active orders on the pair. Additionally it accepts
// an optional GET-parameter limit, which indicates how many orders should be
// displayed (150 by default). Is set to less than 2000.
func (l *Liqui) GetDepth(currencyPair string) (Orderbook, error) {
	type Response struct {
		Data map[string]Orderbook
	}

	response := Response{}
	req := fmt.Sprintf("%s/%s/%s/%s", liquiAPIPublicURL, liquiAPIPublicVersion, liquiDepth, currencyPair)

	return response.Data[currencyPair],
		common.SendHTTPGetRequest(req, true, l.Verbose, &response.Data)
}

// GetTrades returns information about the last trades. Additionally it accepts
// an optional GET-parameter limit, which indicates how many orders should be
// displayed (150 by default). The maximum allowable value is 2000.
func (l *Liqui) GetTrades(currencyPair string) ([]Trades, error) {
	type Response struct {
		Data map[string][]Trades
	}

	response := Response{}
	req := fmt.Sprintf("%s/%s/%s/%s", liquiAPIPublicURL, liquiAPIPublicVersion, liquiTrades, currencyPair)

	return response.Data[currencyPair],
		common.SendHTTPGetRequest(req, true, l.Verbose, &response.Data)
}

// GetAccountInfo returns information about the user’s current balance, API-key
// privileges, the number of open orders and Server Time. To use this method you
// need a privilege of the key info.
func (l *Liqui) GetAccountInfo() (AccountInfo, error) {
	var result AccountInfo

	return result,
		l.SendAuthenticatedHTTPRequest(liquiAccountInfo, url.Values{}, &result)
}

// Trade creates orders on the exchange.
// to-do: convert orderid to int64
func (l *Liqui) Trade(pair, orderType string, amount, price float64) (int64, error) {
	req := url.Values{}
	req.Add("pair", pair)
	req.Add("type", orderType)
	req.Add("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	req.Add("rate", strconv.FormatFloat(price, 'f', -1, 64))

	var result Trade

	return result.OrderID, l.SendAuthenticatedHTTPRequest(liquiTrade, req, &result)
}

// getActiveOrders returns the list of your active orders.
func (l *Liqui) GetActiveOrders(pair string) (map[string]*OrderInfo, error) {
	req := url.Values{}
	req.Add("pair", pair)

	var result map[string]*OrderInfo
	return result, l.SendAuthenticatedHTTPRequest(liquiActiveOrders, req, &result)
}

// GetOrderInfo returns the information on particular order.
func (l *Liqui) GetOrderInfo(orderID string) (map[string]*OrderInfo, error) {
	req := url.Values{}
	req.Add("order_id", orderID)

	var result map[string]*OrderInfo
	return result, l.SendAuthenticatedHTTPRequest(liquiOrderInfo, req, &result)
}

func (l *Liqui) GetOrder(orderID string, currencyPair pair.CurrencyPair) (*exchange.Order, error) {
	orderinfo, err := l.GetOrderInfo(orderID)
	if err != nil {
		return nil, err
	}
	return l.convertOrderToExchangeOrder(orderID, orderinfo[orderID]), nil
}

// Returns the ID of the new exchange order, or an empty string if the order was filled immediately.
func (l *Liqui) NewOrder(symbol pair.CurrencyPair, amount, price float64, side exchange.OrderSide, ordertype exchange.OrderType) (string, error) {
	exchSymbol := exchange.FormatExchangeCurrency(l.Name, symbol).String()
	o64, err := l.Trade(exchSymbol, string(side), amount, price)
	if err != nil {
		return "", err
	}
	if o64 == 0 {
		// If the order is filled immediately Liqui may not bother generating an ID for it and will
		// just return zero.
		return "", nil
	}
	return strconv.FormatInt(o64, 10), nil
}

func (l *Liqui) convertOrderToExchangeOrder(orderID string, order *OrderInfo) *exchange.Order {
	retOrder := &exchange.Order{}
	retOrder.OrderID = orderID

	switch order.Status {
	case 0:
		retOrder.Status = exchange.OrderStatusActive
	case 1: // Executed
		retOrder.Status = exchange.OrderStatusFilled
	case 2, 3: // Canceled, or Canceled and partially executed
		retOrder.Status = exchange.OrderStatusAborted
	}

	// StartAmount is only set for orders returned by GetOrder(),
	// the ones returned by GetOrders() won't have it set.
	if order.StartAmount != 0 {
		amountFilled, _ := decimal.NewFromFloat(order.StartAmount).Sub(decimal.NewFromFloat(order.Amount)).Float64()
		retOrder.Amount = order.StartAmount
		retOrder.FilledAmount = amountFilled
		retOrder.RemainingAmount = order.Amount
	} else {
		retOrder.Amount = order.Amount
	}
	retOrder.Rate = order.Rate
	retOrder.CreatedAt = order.TimestampCreated
	retOrder.CurrencyPair = pair.NewCurrencyPairDelimiter(order.Pair, l.RequestCurrencyPairFormat.Delimiter)
	retOrder.Side = exchange.OrderSide(order.Type) //no conversion neccessary this exchange uses the word buy/sell
	return retOrder
}

func (l *Liqui) GetOrders(pairs []pair.CurrencyPair) ([]*exchange.Order, error) {
	ret := []*exchange.Order{}
	// TODO: filter out orders that don't match the given pairs
	activeorders, err := l.GetActiveOrders("")
	if err != nil {
		return ret, err
	}
	for orderID, order := range activeorders {
		retOrder := l.convertOrderToExchangeOrder(orderID, order)
		ret = append(ret, retOrder)
	}
	return ret, nil
}

// CancelOrder method is used for order cancelation.
func (l *Liqui) CancelOrder(OrderID string, currencyPair pair.CurrencyPair) error {
	req := url.Values{}
	req.Add("order_id", OrderID)

	var result CancelOrder

	err := l.SendAuthenticatedHTTPRequest(liquiCancelOrder, req, &result)
	if err != nil {
		return err
	}

	return nil
}

// GetTradeHistory returns trade history
func (l *Liqui) GetTradeHistory(vals url.Values, pair string) (map[string]TradeHistory, error) {
	if pair != "" {
		vals.Add("pair", pair)
	}

	var result map[string]TradeHistory
	return result, l.SendAuthenticatedHTTPRequest(liquiTradeHistory, vals, &result)
}

// WithdrawCoins is designed for cryptocurrency withdrawals.
// API mentions that this isn't active now, but will be soon - you must provide the first 8 characters of the key
// in your ticket to support.
func (l *Liqui) WithdrawCoins(coin string, amount float64, address string) (WithdrawCoins, error) {
	req := url.Values{}
	req.Add("coinName", coin)
	req.Add("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	req.Add("address", address)

	var result WithdrawCoins
	return result, l.SendAuthenticatedHTTPRequest(liquiWithdrawCoin, req, &result)
}

// SendAuthenticatedHTTPRequest sends an authenticated http request to liqui
func (l *Liqui) SendAuthenticatedHTTPRequest(method string, values url.Values, result interface{}) (err error) {
	if !l.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, l.Name)
	}

	if l.Nonce.Get() == 0 {
		l.Nonce.Set(time.Now().Unix())
	} else {
		l.Nonce.Inc()
	}
	values.Set("nonce", l.Nonce.String())
	values.Set("method", method)

	encoded := values.Encode()
	hmac := common.GetHMAC(common.HashSHA512, []byte(encoded), []byte(l.APISecret))

	if l.Verbose {
		log.Printf("Sending POST request to %s calling method %s with params %s\n", liquiAPIPrivateURL, method, encoded)
	}

	headers := make(map[string]string)
	headers["Key"] = l.APIKey
	headers["Sign"] = common.HexEncodeToString(hmac)
	headers["Content-Type"] = "application/x-www-form-urlencoded"

	resp, err := common.SendHTTPRequest("POST", liquiAPIPrivateURL, headers, strings.NewReader(encoded))
	if err != nil {
		return err
	}

	response := Response{}

	err = common.JSONDecode([]byte(resp), &response)
	if err != nil {
		return err
	}

	if response.Success != 1 {
		return errors.New(response.Error)
	}

	jsonEncoded, err := common.JSONEncode(response.Return)
	if err != nil {
		return err
	}

	err = common.JSONDecode(jsonEncoded, &result)
	if err != nil {
		return err
	}

	return nil
}
