package bittrex

import (
	"encoding/json"
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
	bittrexAPIURL              = "https://bittrex.com/api/v1.1"
	bittrexAPIVersion          = "v1.1"
	bittrexMaxOpenOrders       = 500
	bittrexMaxOrderCountPerDay = 200000
	bittrexTimeFormat          = "2006-01-02T15:04:05"

	// Returned messages from Bittrex API
	bittrexAddressGenerating      = "ADDRESS_GENERATING"
	bittrexErrorMarketNotProvided = "MARKET_NOT_PROVIDED"
	bittrexErrorInvalidMarket     = "INVALID_MARKET"
	bittrexErrorAPIKeyInvalid     = "APIKEY_INVALID"
	bittrexErrorInvalidPermission = "INVALID_PERMISSION"

	// Public requests
	bittrexAPIGetMarkets         = "public/getmarkets"
	bittrexAPIGetCurrencies      = "public/getcurrencies"
	bittrexAPIGetTicker          = "public/getticker"
	bittrexAPIGetMarketSummaries = "public/getmarketsummaries"
	bittrexAPIGetMarketSummary   = "public/getmarketsummary"
	bittrexAPIGetOrderbook       = "public/getorderbook"
	bittrexAPIGetMarketHistory   = "public/getmarkethistory"

	// Market requests
	bittrexAPIBuyLimit      = "market/buylimit"
	bittrexAPISellLimit     = "market/selllimit"
	bittrexAPICancel        = "market/cancel"
	bittrexAPIGetOpenOrders = "market/getopenorders"

	// Account requests
	bittrexAPIGetBalances          = "account/getbalances"
	bittrexAPIGetBalance           = "account/getbalance"
	bittrexAPIGetDepositAddress    = "account/getdepositaddress"
	bittrexAPIWithdraw             = "account/withdraw"
	bittrexAPIGetOrder             = "account/getorder"
	bittrexAPIGetOrderHistory      = "account/getorderhistory"
	bittrexAPIGetWithdrawalHistory = "account/getwithdrawalhistory"
	bittrexAPIGetDepositHistory    = "account/getdeposithistory"
)

// Bittrex is the overaching type across the bittrex methods
type Bittrex struct {
	exchange.Base
	// Maps symbol (exchange specific market identifier) to currency pair info
	currencyPairs map[pair.CurrencyItem]*exchange.CurrencyPairInfo
	// Maps currency pair to min trade size (in base/first currency in the pair)
	minTradeSizes map[pair.CurrencyItem]float64
}

// SetDefaults method assignes the default values for Bittrex
func (b *Bittrex) SetDefaults() {
	b.Name = "Bittrex"
	b.Enabled = false
	b.Verbose = false
	b.Websocket = false
	b.RESTPollingDelay = 10
	b.RequestCurrencyPairFormat.Delimiter = "-"
	b.RequestCurrencyPairFormat.Uppercase = true
	b.ConfigCurrencyPairFormat.Delimiter = "-"
	b.ConfigCurrencyPairFormat.Uppercase = true
	b.AssetTypes = []string{ticker.Spot}
	b.Orderbooks = orderbook.Init()
}

// Setup method sets current configuration details if enabled
func (b *Bittrex) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		b.SetEnabled(false)
	} else {
		b.Enabled = true
		b.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		b.SetAPIKeys(exch.APIKey, exch.APISecret, exch.ClientID, false)
		b.RESTPollingDelay = exch.RESTPollingDelay
		b.Verbose = exch.Verbose
		b.Websocket = exch.Websocket
		b.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
		// Bittrex doesn't follow common conventions for currency pairs, it inverts the
		// currencies for some bizare reason. The currency pairs in the config file should really
		// be called symbols (exchange specific market identifiers), and they'll be converted
		// to currency pairs that follow common conventions as needed.
		b.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
		b.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")
		err := b.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = b.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

// CurrencyPairToSymbol converts a currency pair to a symbol (exchange specific market identifier).
func (b *Bittrex) CurrencyPairToSymbol(p pair.CurrencyPair) string {
	return p.
		// Bittrex symbols are inverted currency pairs
		Invert().
		Display(b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase).
		String()
}

// SymbolToCurrencyPair converts a symbol (exchange specific market identifier) to a currency pair.
func (b *Bittrex) SymbolToCurrencyPair(symbol string) pair.CurrencyPair {
	p := pair.NewCurrencyPairDelimiter(symbol, b.RequestCurrencyPairFormat.Delimiter)
	// Bittrex symbols are inverted currency pairs, so invert them here to get a proper currency pair
	return p.Invert()
}

type currencyLimits struct {
	exchangeName string
	// Maps currency pair to min trade size (in base/first currency in the pair)
	minTradeSizes map[pair.CurrencyItem]float64
}

func newCurrencyLimits(exchangeName string, minTradeSizes map[pair.CurrencyItem]float64) *currencyLimits {
	return &currencyLimits{exchangeName, minTradeSizes}
}

// Returns max number of decimal places allowed in the trade price for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (cl *currencyLimits) GetPriceDecimalPlaces(p pair.CurrencyPair) int32 {
	// API docs don't mention anything about this so make an educated guess...
	return 8
}

// Returns max number of decimal places allowed in the trade amount for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (cl *currencyLimits) GetAmountDecimalPlaces(p pair.CurrencyPair) int32 {
	// API docs don't mention anything about this so make an educated guess...
	return 8
}

// Returns the minimum trade amount for the given currency pair.
func (cl *currencyLimits) GetMinAmount(p pair.CurrencyPair) float64 {
	k := p.Display("/", false)
	if v, exists := cl.minTradeSizes[k]; exists {
		return v
	}
	return 0
}

// GetLimits returns price/amount limits for the exchange.
func (b *Bittrex) GetLimits() exchange.ILimits {
	return newCurrencyLimits(b.Name, b.minTradeSizes)
}

// Returns currency pairs that can be used by the exchange account associated with this bot.
// Use FormatExchangeCurrency to get the right key.
func (b *Bittrex) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	return b.currencyPairs
}

// GetMarkets is used to get the open and available trading markets at Bittrex
// along with other meta data.
func (b *Bittrex) GetMarkets() ([]Market, error) {
	var markets []Market
	path := fmt.Sprintf("%s/%s/", bittrexAPIURL, bittrexAPIGetMarkets)

	return markets, b.HTTPRequest(path, false, url.Values{}, &markets)
}

// GetCurrencies is used to get all supported currencies at Bittrex
func (b *Bittrex) GetCurrencies() ([]Currency, error) {
	var currencies []Currency
	path := fmt.Sprintf("%s/%s/", bittrexAPIURL, bittrexAPIGetCurrencies)

	return currencies, b.HTTPRequest(path, false, url.Values{}, &currencies)
}

// GetTicker sends a public get request and returns current ticker information
// on the supplied currency. Example currency input param "btc-ltc".
func (b *Bittrex) GetTicker(currencyPair string) (Ticker, error) {
	ticker := Ticker{}
	path := fmt.Sprintf("%s/%s?market=%s", bittrexAPIURL, bittrexAPIGetTicker,
		common.StringToUpper(currencyPair),
	)
	return ticker, b.HTTPRequest(path, false, url.Values{}, &ticker)
}

// GetMarketSummaries is used to get the last 24 hour summary of all active
// exchanges
func (b *Bittrex) GetMarketSummaries() ([]MarketSummary, error) {
	var summaries []MarketSummary
	path := fmt.Sprintf("%s/%s/", bittrexAPIURL, bittrexAPIGetMarketSummaries)

	return summaries, b.HTTPRequest(path, false, url.Values{}, &summaries)
}

// GetMarketSummary is used to get the last 24 hour summary of all active
// exchanges by currency pair (btc-ltc).
func (b *Bittrex) GetMarketSummary(currencyPair string) ([]MarketSummary, error) {
	var summary []MarketSummary
	path := fmt.Sprintf("%s/%s?market=%s", bittrexAPIURL,
		bittrexAPIGetMarketSummary, common.StringToLower(currencyPair),
	)
	return summary, b.HTTPRequest(path, false, url.Values{}, &summary)
}

// GetOrderbook method returns current order book information by currency, type
// & depth.
// "Currency Pair" ie btc-ltc
// "Category" either "buy", "sell" or "both"; for ease of use and reduced
// complexity this function is set to "both"
// "Depth" max depth is 50 but you can literally set it any integer you want and
// it returns full depth. So depth default is 50.
func (b *Bittrex) GetOrderbook(currencyPair string) (OrderBooks, error) {
	var orderbooks OrderBooks
	path := fmt.Sprintf("%s/%s?market=%s&type=both&depth=50", bittrexAPIURL,
		bittrexAPIGetOrderbook, common.StringToUpper(currencyPair),
	)

	return orderbooks, b.HTTPRequest(path, false, url.Values{}, &orderbooks)
}

// GetMarketHistory retrieves the latest trades that have occurred for a specific
// market
func (b *Bittrex) GetMarketHistory(currencyPair string) ([]MarketHistory, error) {
	var marketHistoriae []MarketHistory
	path := fmt.Sprintf("%s/%s?market=%s", bittrexAPIURL,
		bittrexAPIGetMarketHistory, common.StringToUpper(currencyPair),
	)
	return marketHistoriae, b.HTTPRequest(path, false, url.Values{},
		&marketHistoriae)
}

// PlaceBuyLimit is used to place a buy order in a specific market. Use buylimit
// to place limit orders. Make sure you have the proper permissions set on your
// API keys for this call to work.
// "Currency" ie "btc-ltc"
// "Quantity" is the amount to purchase
// "Rate" is the rate at which to purchase
func (b *Bittrex) PlaceBuyLimit(currencyPair string, quantity, rate float64) (string, error) {
	var response UUID
	values := url.Values{}
	values.Set("market", currencyPair)
	values.Set("quantity", strconv.FormatFloat(quantity, 'E', -1, 64))
	values.Set("rate", strconv.FormatFloat(rate, 'E', -1, 64))
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIBuyLimit)

	return response.ID, b.HTTPRequest(path, true, values, &response)
}

// PlaceSellLimit is used to place a sell order in a specific market. Use
// selllimit to place limit orders. Make sure you have the proper permissions
// set on your API keys for this call to work.
// "Currency" ie "btc-ltc"
// "Quantity" is the amount to purchase
// "Rate" is the rate at which to purchase
func (b *Bittrex) PlaceSellLimit(currencyPair string, quantity, rate float64) (string, error) {
	var response UUID
	values := url.Values{}
	values.Set("market", currencyPair)
	values.Set("quantity", strconv.FormatFloat(quantity, 'E', -1, 64))
	values.Set("rate", strconv.FormatFloat(rate, 'E', -1, 64))
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPISellLimit)

	return response.ID, b.HTTPRequest(path, true, values, &response)
}

// GetOpenOrders returns all orders that you currently have opened.
// A specific market can be requested for example "btc-ltc"
func (b *Bittrex) GetOpenOrders(currencyPair string) ([]Order, error) {
	var orders []Order
	values := url.Values{}
	if !(currencyPair == "" || currencyPair == " ") {
		values.Set("market", currencyPair)
	}
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetOpenOrders)

	return orders, b.HTTPRequest(path, true, values, &orders)
}

func (b *Bittrex) CancelOrder(uuid string) error {
	_, err := b.cancelOrder(uuid)
	return err
}

func (b *Bittrex) GetOrder(orderID string) (*exchange.Order, error) {
	order, err := b.getOrder(orderID)
	if err != nil {
		return nil, err
	}
	retOrder := b.convertOrderToExchangeOrder(orderID, &order)
	return retOrder, nil
}

func (b *Bittrex) convertOrderToExchangeOrder(orderID string, order *Order) *exchange.Order {
	ll := log.WithField("exchange", b.Name).WithField("orderID", orderID)
	retOrder := &exchange.Order{}
	retOrder.OrderID = order.OrderUUID

	if len(order.Closed) > 0 {
		if order.QuantityRemaining > 0 {
			retOrder.Status = exchange.OrderStatusAborted
		} else {
			retOrder.Status = exchange.OrderStatusFilled
		}
	} else {
		retOrder.Status = exchange.OrderStatusActive
	}

	var isExact bool
	if retOrder.FilledAmount, isExact = decimal.NewFromFloat(order.Quantity).
		Sub(decimal.NewFromFloat(order.QuantityRemaining)).Float64(); !isExact {
		ll.Warnf("conversion of filled amount to float64 was inexact")
	}
	retOrder.RemainingAmount = order.QuantityRemaining
	retOrder.Amount = order.Quantity

	// Bittrex doesn't seem to set the PricePerUnit field for orders returned from
	// /market/getopenorders but it does seem to set the Limit field (for limit buy/sell at least).
	if order.PricePerUnit > 0 {
		retOrder.Rate = order.PricePerUnit
	} else {
		retOrder.Rate = order.Limit
	}

	createdAt, err := time.Parse(bittrexTimeFormat, order.Opened)
	if err != nil {
		ll.WithError(err).Errorf("failed to parse %s", order.Opened)
	} else {
		retOrder.CreatedAt = createdAt.Unix()
	}

	retOrder.CurrencyPair = b.SymbolToCurrencyPair(order.Exchange)

	if order.Type == "LIMIT_BUY" {
		retOrder.Side = exchange.OrderSideBuy
	} else if order.Type == "LIMIT_SELL" {
		retOrder.Side = exchange.OrderSideSell
	} else {
		ll.Errorf("failed to convert '%s' to order side", order.Type)
	}

	return retOrder
}

func (b *Bittrex) NewOrder(
	currencyPair pair.CurrencyPair, amount, price float64, side exchange.OrderSide,
	ordertype exchange.OrderType) (string, error) {
	symbol := b.CurrencyPairToSymbol(currencyPair)
	var orderID string
	var err error
	if side == exchange.OrderSideBuy {
		orderID, err = b.PlaceBuyLimit(symbol, amount, price)
	} else if side == exchange.OrderSideSell {
		orderID, err = b.PlaceSellLimit(symbol, amount, price)
	} else {
		return "", fmt.Errorf("can't create order on %s exchange invalid value '%s' for side", b.Name, side)
	}

	if err != nil {
		return "", err
	}
	return orderID, err
}

func (b *Bittrex) GetOrders() ([]*exchange.Order, error) {
	ret := []*exchange.Order{}

	orders, err := b.GetOpenOrders("")
	if err != nil {
		return ret, err
	}

	for _, order := range orders {
		ret = append(ret, b.convertOrderToExchangeOrder(order.OrderUUID, &order))
	}
	return ret, nil
}

// CancelOrder is used to cancel a buy or sell order.
func (b *Bittrex) cancelOrder(uuid string) ([]Balance, error) {
	var balances []Balance
	values := url.Values{}
	values.Set("uuid", uuid)
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPICancel)

	return balances, b.HTTPRequest(path, true, values, &balances)
}

// GetAccountBalances is used to retrieve all balances from your account
func (b *Bittrex) GetAccountBalances() ([]Balance, error) {
	var balances []Balance
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetBalances)

	return balances, b.HTTPRequest(path, true, url.Values{}, &balances)
}

// GetAccountBalanceByCurrency is used to retrieve the balance from your account
// for a specific currency. ie. "btc" or "ltc"
func (b *Bittrex) GetAccountBalanceByCurrency(currency string) (Balance, error) {
	var balance Balance
	values := url.Values{}
	values.Set("currency", currency)
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetBalance)

	return balance, b.HTTPRequest(path, true, values, &balance)
}

// GetDepositAddress is used to retrieve or generate an address for a specific
// currency. If one does not exist, the call will fail and return
// ADDRESS_GENERATING until one is available.
func (b *Bittrex) GetDepositAddress(currency string) (DepositAddress, error) {
	var address DepositAddress
	values := url.Values{}
	values.Set("currency", currency)
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetDepositAddress)

	return address, b.HTTPRequest(path, true, values, &address)
}

// Withdraw is used to withdraw funds from your account.
// note: Please account for transaction fee.
func (b *Bittrex) Withdraw(currency, paymentID, address string, quantity float64) (UUID, error) {
	var id UUID
	values := url.Values{}
	values.Set("currency", currency)
	values.Set("quantity", strconv.FormatFloat(quantity, 'E', -1, 64))
	values.Set("address", address)
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIWithdraw)

	return id, b.HTTPRequest(path, true, values, &id)
}

// GetOrder is used to retrieve a single order by UUID.
func (b *Bittrex) getOrder(uuid string) (Order, error) {
	var order Order
	values := url.Values{}
	values.Set("uuid", uuid)
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetOrder)

	msg, err := b.HTTPRequestJSON(path, true, values)
	if err != nil {
		return order, err
	}
	if err = json.Unmarshal(msg, &order); err != nil {
		return order, err
	}

	// On orders returned by the /market/getorder endpoint Bittrex stores the order type in a
	// field named Type instead of OrderType (like for other endpoints).
	type OrderType struct {
		Type string
	}
	if order.Type == "" {
		var orderType OrderType
		if err = json.Unmarshal(msg, &orderType); err != nil {
			return order, err
		}
		order.Type = orderType.Type
	}
	return order, nil
}

// GetOrderHistory is used to retrieve your order history. If currencyPair
// omitted it will return the entire order History.
func (b *Bittrex) GetOrderHistory(currencyPair string) ([]Order, error) {
	var orders []Order
	values := url.Values{}

	if !(currencyPair == "" || currencyPair == " ") {
		values.Set("market", currencyPair)
	}
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetOrderHistory)

	return orders, b.HTTPRequest(path, true, values, &orders)
}

// GetWithdrawalHistory is used to retrieve your withdrawal history. If currency
// omitted it will return the entire history
func (b *Bittrex) GetWithdrawalHistory(currency string) ([]WithdrawalHistory, error) {
	var history []WithdrawalHistory
	values := url.Values{}

	if !(currency == "" || currency == " ") {
		values.Set("currency", currency)
	}
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetWithdrawalHistory)

	return history, b.HTTPRequest(path, true, values, &history)
}

// GetDepositHistory is used to retrieve your deposit history. If currency is
// is omitted it will return the entire deposit history
func (b *Bittrex) GetDepositHistory(currency string) ([]WithdrawalHistory, error) {
	var history []WithdrawalHistory
	values := url.Values{}

	if !(currency == "" || currency == " ") {
		values.Set("currency", currency)
	}
	path := fmt.Sprintf("%s/%s", bittrexAPIURL, bittrexAPIGetDepositHistory)

	return history, b.HTTPRequest(path, true, values, &history)
}

// SendAuthenticatedHTTPRequest sends an authenticated http request to a desired
// path
func (b *Bittrex) SendAuthenticatedHTTPRequest(path string, values url.Values, result interface{}) (err error) {
	if !b.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, b.Name)
	}

	if b.Nonce.Get() == 0 {
		b.Nonce.Set(time.Now().UnixNano())
	} else {
		b.Nonce.Inc()
	}
	values.Set("apikey", b.APIKey)
	values.Set("apisecret", b.APISecret)
	values.Set("nonce", b.Nonce.String())
	rawQuery := path + "?" + values.Encode()
	hmac := common.GetHMAC(
		common.HashSHA512, []byte(rawQuery), []byte(b.APISecret),
	)
	headers := make(map[string]string)
	headers["apisign"] = common.HexEncodeToString(hmac)

	resp, err := common.SendHTTPRequest(
		"GET", rawQuery, headers, strings.NewReader(""),
	)
	if err != nil {
		return err
	}

	if b.Verbose {
		log.Printf("Received raw: %s\n", resp)
	}

	err = common.JSONDecode([]byte(resp), &result)
	if err != nil {
		return errors.New("Unable to JSON Unmarshal response." + err.Error())
	}
	return nil
}

// HTTPRequest sends an HTTP request to a Bittrex API endpoint and and returns the result as raw JSON.
func (b *Bittrex) HTTPRequestJSON(path string, auth bool, values url.Values) (json.RawMessage, error) {
	response := Response{}
	if auth {
		if err := b.SendAuthenticatedHTTPRequest(path, values, &response); err != nil {
			return nil, err
		}
	} else {
		if err := common.SendHTTPGetRequest(path, true, b.Verbose, &response); err != nil {
			return nil, err
		}
	}
	if response.Success {
		return response.Result, nil
	}
	return nil, errors.New(response.Message)
}

// HTTPRequest is a generalised http request function.
func (b *Bittrex) HTTPRequest(path string, auth bool, values url.Values, v interface{}) error {
	msg, err := b.HTTPRequestJSON(path, auth, values)
	if err != nil {
		return err
	}
	return json.Unmarshal(msg, &v)
}
