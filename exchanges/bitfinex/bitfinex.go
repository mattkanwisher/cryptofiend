package bitfinex

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/config"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
)

const (
	bitfinexAPIURL             = "https://api.bitfinex.com/v1/"
	bitfinexAPIVersion         = "1"
	bitfinexTicker             = "pubticker/"
	bitfinexStats              = "stats/"
	bitfinexLendbook           = "lendbook/"
	bitfinexOrderbook          = "book/"
	bitfinexTrades             = "trades/"
	bitfinexKeyPermissions     = "key_info"
	bitfinexLends              = "lends/"
	bitfinexSymbols            = "symbols/"
	bitfinexSymbolsDetails     = "symbols_details/"
	bitfinexAccountInfo        = "account_infos"
	bitfinexAccountFees        = "account_fees"
	bitfinexAccountSummary     = "summary"
	bitfinexDeposit            = "deposit/new"
	bitfinexOrderNew           = "order/new"
	bitfinexOrderNewMulti      = "order/new/multi"
	bitfinexOrderCancel        = "order/cancel"
	bitfinexOrderCancelMulti   = "order/cancel/multi"
	bitfinexOrderCancelAll     = "order/cancel/all"
	bitfinexOrderCancelReplace = "order/cancel/replace"
	bitfinexOrderStatus        = "order/status"
	bitfinexOrders             = "orders"
	bitfinexPositions          = "positions"
	bitfinexClaimPosition      = "position/claim"
	bitfinexHistory            = "history"
	bitfinexHistoryMovements   = "history/movements"
	bitfinexTradeHistory       = "mytrades"
	bitfinexOfferNew           = "offer/new"
	bitfinexOfferCancel        = "offer/cancel"
	bitfinexOfferStatus        = "offer/status"
	bitfinexOffers             = "offers"
	bitfinexMarginActiveFunds  = "taken_funds"
	bitfinexMarginTotalFunds   = "total_taken_funds"
	bitfinexMarginUnusedFunds  = "unused_taken_funds"
	bitfinexMarginClose        = "funding/close"
	bitfinexBalances           = "balances"
	bitfinexMarginInfo         = "margin_infos"
	bitfinexTransfer           = "transfer"
	bitfinexWithdrawal         = "withdraw"
	bitfinexActiveCredits      = "credits"

	// bitfinexMaxRequests if exceeded IP address blocked 10-60 sec, JSON response
	// {"error": "ERR_RATE_LIMIT"}
	bitfinexMaxRequests = 90
)

type rateLimitInfo struct {
	StartTime    int64
	RequestCount uint
}

// Bitfinex is the overarching type across the bitfinex package
// Notes: Bitfinex has added a rate limit to the number of REST requests.
// Rate limit policy can vary in a range of 10 to 90 requests per minute
// depending on some factors (e.g. servers load, endpoint, etc.).
type Bitfinex struct {
	exchange.Base
	WebsocketConn         *websocket.Conn
	WebsocketSubdChannels map[int]WebsocketChanInfo
	// Maps symbol (exchange specific market identifier) to currency pair info
	currencyPairs map[pair.CurrencyItem]*exchange.CurrencyPairInfo
	symbolDetails map[pair.CurrencyItem]*SymbolDetails
	rateLimits    map[string]*rateLimitInfo
	// Cached stuff that's behind rate limited REST API endpoints
	lastBalances []Balance
}

// SetDefaults sets the basic defaults for bitfinex
func (b *Bitfinex) SetDefaults() {
	b.Name = "Bitfinex"
	b.Enabled = false
	b.Verbose = false
	b.Websocket = false
	b.RESTPollingDelay = 10
	b.WebsocketSubdChannels = make(map[int]WebsocketChanInfo)
	b.RequestCurrencyPairFormat.Delimiter = ""
	b.RequestCurrencyPairFormat.Uppercase = true
	b.ConfigCurrencyPairFormat.Delimiter = ""
	b.ConfigCurrencyPairFormat.Uppercase = true
	b.AssetTypes = []string{ticker.Spot}
	b.Orderbooks = orderbook.Init()
	b.rateLimits = map[string]*rateLimitInfo{}
	b.lastBalances = []Balance{}
}

// Setup takes in the supplied exchange configuration details and sets params
func (b *Bitfinex) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		b.SetEnabled(false)
	} else {
		b.Enabled = true
		b.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		b.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		b.RESTPollingDelay = exch.RESTPollingDelay
		b.Verbose = exch.Verbose
		b.Websocket = exch.Websocket
		b.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
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
func (b *Bitfinex) CurrencyPairToSymbol(p pair.CurrencyPair) string {
	return p.
		Display(b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase).
		String()
}

// SymbolToCurrencyPair converts a symbol (exchange specific market identifier) to a currency pair.
func (b *Bitfinex) SymbolToCurrencyPair(symbol string) (pair.CurrencyPair, error) {
	if len(symbol) != 6 {
		return pair.CurrencyPair{}, fmt.Errorf("symbol %s is longer than expected", symbol)
	}
	p := pair.NewCurrencyPair(symbol[0:3], symbol[3:])
	return p.FormatPair(b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase), nil
}

type currencyLimits struct {
	exchangeName string
	// Maps symbol (lower-case) to symbol details
	data map[pair.CurrencyItem]*SymbolDetails
}

func newCurrencyLimits(exchangeName string, data map[pair.CurrencyItem]*SymbolDetails) *currencyLimits {
	return &currencyLimits{exchangeName, data}
}

// Returns max number of decimal places allowed in the trade price for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (cl *currencyLimits) GetPriceDecimalPlaces(p pair.CurrencyPair) int32 {
	k := p.Display("/", false)
	if v, exists := cl.data[k]; exists {
		return int32(v.PricePrecision)
	}
	return 0
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
	if v, exists := cl.data[k]; exists {
		return v.MinimumOrderSize
	}
	return 0
}

// GetLimits returns price/amount limits for the exchange.
func (b *Bitfinex) GetLimits() exchange.ILimits {
	return newCurrencyLimits(b.Name, b.symbolDetails)
}

// Returns currency pairs that can be used by the exchange account associated with this bot.
// Use FormatExchangeCurrency to get the right key.
func (b *Bitfinex) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	return b.currencyPairs
}

// GetTicker returns ticker information
func (b *Bitfinex) GetTicker(symbol string, values url.Values) (Ticker, error) {
	response := Ticker{}
	path := common.EncodeURLValues(bitfinexAPIURL+bitfinexTicker+symbol, values)

	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetStats returns various statistics about the requested pair
func (b *Bitfinex) GetStats(symbol string) ([]Stat, error) {
	response := []Stat{}
	path := fmt.Sprint(bitfinexAPIURL + bitfinexStats + symbol)

	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetFundingBook the entire margin funding book for both bids and asks sides
// per currency string
// symbol - example "USD"
func (b *Bitfinex) GetFundingBook(symbol string) (FundingBook, error) {
	response := FundingBook{}
	path := fmt.Sprint(bitfinexAPIURL + bitfinexLendbook + symbol)

	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetOrderbook retieves the orderbook bid and ask price points for a currency
// pair - By default the response will return 25 bid and 25 ask price points.
// CurrencyPair - Example "BTCUSD"
// Values can contain limit amounts for both the asks and bids - Example
// "limit_bids" = 1000
func (b *Bitfinex) GetOrderbook(currencyPair string, values url.Values) (Orderbook, error) {
	response := Orderbook{}
	path := common.EncodeURLValues(
		bitfinexAPIURL+bitfinexOrderbook+currencyPair,
		values,
	)
	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetTrades returns a list of the most recent trades for the given curencyPair
// By default the response will return 100 trades
// CurrencyPair - Example "BTCUSD"
// Values can contain limit amounts for the number of trades returned - Example
// "limit_trades" = 1000
func (b *Bitfinex) GetTrades(currencyPair string, values url.Values) ([]TradeStructure, error) {
	response := []TradeStructure{}
	path := common.EncodeURLValues(
		bitfinexAPIURL+bitfinexTrades+currencyPair,
		values,
	)
	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetLendbook returns a list of the most recent funding data for the given
// currency: total amount provided and Flash Return Rate (in % by 365 days) over
// time
// Symbol - example "USD"
func (b *Bitfinex) GetLendbook(symbol string, values url.Values) (Lendbook, error) {
	response := Lendbook{}
	if len(symbol) == 6 {
		symbol = symbol[:3]
	}
	path := common.EncodeURLValues(bitfinexAPIURL+bitfinexLendbook+symbol, values)

	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetLends returns a list of the most recent funding data for the given
// currency: total amount provided and Flash Return Rate (in % by 365 days)
// over time
// Symbol - example "USD"
func (b *Bitfinex) GetLends(symbol string, values url.Values) ([]Lends, error) {
	response := []Lends{}
	path := common.EncodeURLValues(bitfinexAPIURL+bitfinexLends+symbol, values)

	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetSymbols returns the available currency pairs on the exchange
func (b *Bitfinex) GetSymbols() ([]string, error) {
	products := []string{}
	path := fmt.Sprint(bitfinexAPIURL + bitfinexSymbols)

	return products, common.SendHTTPGetRequest(path, true, b.Verbose, &products)
}

// GetSymbolsDetails a list of valid symbol IDs and the pair details
func (b *Bitfinex) GetSymbolsDetails() ([]SymbolDetails, error) {
	response := []SymbolDetails{}
	path := fmt.Sprint(bitfinexAPIURL + bitfinexSymbolsDetails)

	return response, common.SendHTTPGetRequest(path, true, b.Verbose, &response)
}

// GetAccountInfo returns information about your account incl. trading fees
func (b *Bitfinex) GetAccountInfo() ([]AccountInfo, error) {
	response := []AccountInfo{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexAccountInfo, nil, &response)
}

// GetAccountFees - NOT YET IMPLEMENTED
func (b *Bitfinex) GetAccountFees() (AccountFees, error) {
	response := AccountFees{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexAccountFees, nil, &response)
}

// GetAccountSummary returns a 30-day summary of your trading volume and return
// on margin funding
func (b *Bitfinex) GetAccountSummary() (AccountSummary, error) {
	response := AccountSummary{}

	return response,
		b.SendAuthenticatedHTTPRequest(
			"POST", bitfinexAccountSummary, nil, &response,
		)
}

// NewDeposit returns a new deposit address
// Method - Example methods accepted: “bitcoin”, “litecoin”, “ethereum”,
//“tethers", "ethereumc", "zcash", "monero", "iota", "bcash"
// WalletName - accepted: “trading”, “exchange”, “deposit”
// renew - Default is 0. If set to 1, will return a new unused deposit address
func (b *Bitfinex) NewDeposit(method, walletName string, renew int) (DepositResponse, error) {
	response := DepositResponse{}
	request := make(map[string]interface{})
	request["method"] = method
	request["wallet_name"] = walletName
	request["renew"] = renew

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexDeposit, request, &response)
}

// GetKeyPermissions checks the permissions of the key being used to generate
// this request.
func (b *Bitfinex) GetKeyPermissions() (KeyPermissions, error) {
	response := KeyPermissions{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexKeyPermissions, nil, &response)
}

// GetMarginInfo shows your trading wallet information for margin trading
func (b *Bitfinex) GetMarginInfo() ([]MarginInfo, error) {
	response := []MarginInfo{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexMarginInfo, nil, &response)
}

// GetAccountBalance returns full wallet balance information
func (b *Bitfinex) GetAccountBalance() ([]Balance, error) {
	response := []Balance{}
	err := b.SendRateLimitedHTTPRequest(20, "POST", bitfinexBalances, nil, &response, b.lastBalances)
	if err != nil {
		return nil, err
	}
	b.lastBalances = response
	return response, nil
}

// WalletTransfer move available balances between your wallets
// Amount - Amount to move
// Currency -  example "BTC"
// WalletFrom - example "exchange"
// WalletTo -  example "deposit"
func (b *Bitfinex) WalletTransfer(amount float64, currency, walletFrom, walletTo string) ([]WalletTransfer, error) {
	response := []WalletTransfer{}
	request := make(map[string]interface{})
	request["amount"] = amount
	request["currency"] = currency
	request["walletfrom"] = walletFrom
	request["walletTo"] = walletTo

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexTransfer, request, &response)
}

// Withdrawal requests a withdrawal from one of your wallets.
// Major Upgrade needed on this function to include all query params
func (b *Bitfinex) Withdrawal(withdrawType, wallet, address string, amount float64) ([]Withdrawal, error) {
	response := []Withdrawal{}
	request := make(map[string]interface{})
	request["withdrawal_type"] = withdrawType
	request["walletselected"] = wallet
	request["amount"] = strconv.FormatFloat(amount, 'f', -1, 64)
	request["address"] = address

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexWithdrawal, request, &response)
}

// newOrder submits a new order and returns a order information
// Major Upgrade needed on this function to include all query params
func (b *Bitfinex) newOrder(symbol string, amount float64, price float64, side, Type string, hidden bool) (Order, error) {
	response := Order{}
	request := make(map[string]interface{})
	request["symbol"] = symbol
	request["amount"] = strconv.FormatFloat(amount, 'f', -1, 64)
	request["price"] = strconv.FormatFloat(price, 'f', -1, 64)
	request["exchange"] = "bitfinex"
	request["type"] = Type
	request["is_hidden"] = hidden
	request["side"] = side // this exchange uses the string buy/sell so no conversion neccessary

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrderNew, request, &response)
}

// NewOrder submits a new order and returns the ID of the new exchange order
func (b *Bitfinex) NewOrder(currencyPair pair.CurrencyPair, amount, price float64,
	side exchange.OrderSide, orderType exchange.OrderType) (string, error) {
	symbol := b.CurrencyPairToSymbol(currencyPair)
	order, err := b.newOrder(symbol, amount, price, string(side), string(orderType), false)
	if err != nil {
		return "", err
	}
	orderID := strconv.FormatInt(order.ID, 10)
	return orderID, nil
}

// NewOrderMulti allows several new orders at once
func (b *Bitfinex) NewOrderMulti(orders []PlaceOrder) (OrderMultiResponse, error) {
	response := OrderMultiResponse{}
	request := make(map[string]interface{})
	request["orders"] = orders

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrderNewMulti, request, &response)
}

func (b *Bitfinex) CancelOrder(orderStr string) error {
	var orderID int64
	var err error
	if orderID, err = strconv.ParseInt(orderStr, 10, 64); err != nil {
		return err
	}
	_, err = b.cancelOrder(orderID)
	return err
}

// CancelOrder cancels a single order
func (b *Bitfinex) cancelOrder(OrderID int64) (Order, error) {
	response := Order{}
	request := make(map[string]interface{})
	request["order_id"] = OrderID

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrderCancel, request, &response)
}

// CancelMultipleOrders cancels multiple orders
func (b *Bitfinex) CancelMultipleOrders(OrderIDs []int64) (string, error) {
	response := GenericResponse{}
	request := make(map[string]interface{})
	request["order_ids"] = OrderIDs

	return response.Result,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrderCancelMulti, request, nil)
}

// CancelAllOrders cancels all active and open orders
func (b *Bitfinex) CancelAllOrders() (string, error) {
	response := GenericResponse{}

	return response.Result,
		b.SendAuthenticatedHTTPRequest("GET", bitfinexOrderCancelAll, nil, nil)
}

// ReplaceOrder replaces an older order with a new order
func (b *Bitfinex) ReplaceOrder(OrderID int64, Symbol string, Amount float64, Price float64, Buy bool, Type string, Hidden bool) (Order, error) {
	response := Order{}
	request := make(map[string]interface{})
	request["order_id"] = OrderID
	request["symbol"] = Symbol
	request["amount"] = strconv.FormatFloat(Amount, 'f', -1, 64)
	request["price"] = strconv.FormatFloat(Price, 'f', -1, 64)
	request["exchange"] = "bitfinex"
	request["type"] = Type
	request["is_hidden"] = Hidden

	if Buy {
		request["side"] = "buy"
	} else {
		request["side"] = "sell"
	}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrderCancelReplace, request, &response)
}

// GetOrderStatus returns order status information
func (b *Bitfinex) GetOrderStatus(OrderID int64) (Order, error) {
	orderStatus := Order{}
	request := make(map[string]interface{})
	request["order_id"] = OrderID

	return orderStatus,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrderStatus, request, &orderStatus)
}

// GetOrder returns information about the exchange order matching the given ID
func (b *Bitfinex) GetOrder(orderID string) (*exchange.Order, error) {
	id, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return nil, err
	}
	order, err := b.GetOrderStatus(id)
	if err != nil {
		return nil, err
	}
	return b.convertOrderToExchangeOrder(&order), nil
}

func (b *Bitfinex) convertOrderToExchangeOrder(order *Order) *exchange.Order {
	retOrder := &exchange.Order{}
	retOrder.OrderID = strconv.FormatInt(order.ID, 10)

	if order.IsCancelled {
		retOrder.Status = exchange.OrderStatusAborted
	} else if order.IsLive {
		retOrder.Status = exchange.OrderStatusActive
	} else {
		retOrder.Status = exchange.OrderStatusFilled
	}

	retOrder.Amount = order.OriginalAmount
	retOrder.FilledAmount = order.ExecutedAmount
	retOrder.RemainingAmount = order.RemainingAmount

	if order.AverageExecutionPrice == 0 {
		retOrder.Rate = order.Price
	} else {
		retOrder.Rate = order.AverageExecutionPrice
	}

	var createdAt int64
	// Drop the fractional part of the timestamp, whatever it is.
	timeParts := strings.Split(order.Timestamp, ".")
	if len(timeParts) > 0 {
		createdAt, _ = strconv.ParseInt(timeParts[0], 10, 64)
	}
	retOrder.CreatedAt = createdAt

	retOrder.CurrencyPair, _ = b.SymbolToCurrencyPair(order.Symbol)
	retOrder.Side = exchange.OrderSide(order.Side)
	retOrder.Type = exchange.OrderType(order.Type)

	return retOrder
}

// GetActiveOrders returns all active orders and statuses
func (b *Bitfinex) GetActiveOrders() ([]Order, error) {
	response := []Order{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrders, nil, &response)
}

func (b *Bitfinex) GetOrders() ([]*exchange.Order, error) {
	orders, err := b.GetActiveOrders()
	if err != nil {
		return nil, err
	}
	ret := make([]*exchange.Order, 0, len(orders))
	for _, order := range orders {
		ret = append(ret, b.convertOrderToExchangeOrder(&order))
	}
	return ret, nil
}

// GetActivePositions returns an array of active positions
func (b *Bitfinex) GetActivePositions() ([]Position, error) {
	response := []Position{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexPositions, nil, &response)
}

// ClaimPosition allows positions to be claimed
func (b *Bitfinex) ClaimPosition(PositionID int) (Position, error) {
	response := Position{}
	request := make(map[string]interface{})
	request["position_id"] = PositionID

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexClaimPosition, nil, nil)
}

// GetBalanceHistory returns balance history for the account
func (b *Bitfinex) GetBalanceHistory(symbol string, timeSince, timeUntil time.Time, limit int, wallet string) ([]BalanceHistory, error) {
	response := []BalanceHistory{}
	request := make(map[string]interface{})
	request["currency"] = symbol

	if !timeSince.IsZero() {
		request["since"] = timeSince
	}
	if !timeUntil.IsZero() {
		request["until"] = timeUntil
	}
	if limit > 0 {
		request["limit"] = limit
	}
	if len(wallet) > 0 {
		request["wallet"] = wallet
	}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexHistory, request, &response)
}

// GetMovementHistory returns an array of past deposits and withdrawals
func (b *Bitfinex) GetMovementHistory(symbol, method string, timeSince, timeUntil time.Time, limit int) ([]MovementHistory, error) {
	response := []MovementHistory{}
	request := make(map[string]interface{})
	request["currency"] = symbol

	if len(method) > 0 {
		request["method"] = method
	}
	if !timeSince.IsZero() {
		request["since"] = timeSince
	}
	if !timeUntil.IsZero() {
		request["until"] = timeUntil
	}
	if limit > 0 {
		request["limit"] = limit
	}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexHistoryMovements, request, &response)
}

// GetTradeHistory returns past executed trades
func (b *Bitfinex) GetTradeHistory(currencyPair string, timestamp, until time.Time, limit, reverse int) ([]TradeHistory, error) {
	response := []TradeHistory{}
	request := make(map[string]interface{})
	request["currency"] = currencyPair
	request["timestamp"] = timestamp

	if !until.IsZero() {
		request["until"] = until
	}
	if limit > 0 {
		request["limit"] = limit
	}
	if reverse > 0 {
		request["reverse"] = reverse
	}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexTradeHistory, request, &response)
}

// NewOffer submits a new offer
func (b *Bitfinex) NewOffer(symbol string, amount, rate float64, period int64, direction string) (Offer, error) {
	response := Offer{}
	request := make(map[string]interface{})
	request["currency"] = symbol
	request["amount"] = amount
	request["rate"] = rate
	request["period"] = period
	request["direction"] = direction

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOfferNew, request, &response)
}

// CancelOffer cancels offer by offerID
func (b *Bitfinex) CancelOffer(OfferID int64) (Offer, error) {
	response := Offer{}
	request := make(map[string]interface{})
	request["offer_id"] = OfferID

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOfferCancel, request, &response)
}

// GetOfferStatus checks offer status whether it has been cancelled, execute or
// is still active
func (b *Bitfinex) GetOfferStatus(OfferID int64) (Offer, error) {
	response := Offer{}
	request := make(map[string]interface{})
	request["offer_id"] = OfferID

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOrderStatus, request, &response)
}

// GetActiveCredits returns all available credits
func (b *Bitfinex) GetActiveCredits() ([]Offer, error) {
	response := []Offer{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexActiveCredits, nil, &response)
}

// GetActiveOffers returns all current active offers
func (b *Bitfinex) GetActiveOffers() ([]Offer, error) {
	response := []Offer{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexOffers, nil, &response)
}

// GetActiveMarginFunding returns an array of active margin funds
func (b *Bitfinex) GetActiveMarginFunding() ([]MarginFunds, error) {
	response := []MarginFunds{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexMarginActiveFunds, nil, &response)
}

// GetUnusedMarginFunds returns an array of funding borrowed but not currently
// used
func (b *Bitfinex) GetUnusedMarginFunds() ([]MarginFunds, error) {
	response := []MarginFunds{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexMarginUnusedFunds, nil, &response)
}

// GetMarginTotalTakenFunds returns an array of active funding used in a
// position
func (b *Bitfinex) GetMarginTotalTakenFunds() ([]MarginTotalTakenFunds, error) {
	response := []MarginTotalTakenFunds{}

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexMarginTotalFunds, nil, &response)
}

// CloseMarginFunding closes an unused or used taken fund
func (b *Bitfinex) CloseMarginFunding(SwapID int64) (Offer, error) {
	response := Offer{}
	request := make(map[string]interface{})
	request["swap_id"] = SwapID

	return response,
		b.SendAuthenticatedHTTPRequest("POST", bitfinexMarginClose, request, &response)
}

// SendAuthenticatedHTTPRequest sends an autheticated http request and json
// unmarshals result to a supplied variable
func (b *Bitfinex) SendAuthenticatedHTTPRequest(method, path string, params map[string]interface{}, result interface{}) error {
	if !b.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, b.Name)
	}

	if b.Nonce.Get() == 0 {
		b.Nonce.Set(time.Now().UnixNano())
	} else {
		b.Nonce.Inc()
	}

	respErr := ErrorCapture{}
	request := make(map[string]interface{})
	request["request"] = fmt.Sprintf("/v%s/%s", bitfinexAPIVersion, path)
	request["nonce"] = b.Nonce.String()

	if params != nil {
		for key, value := range params {
			request[key] = value
		}
	}

	PayloadJSON, err := common.JSONEncode(request)
	if err != nil {
		return errors.New("SendAuthenticatedHTTPRequest: Unable to JSON request")
	}

	if b.Verbose {
		log.Printf("Request JSON: %s\n", PayloadJSON)
	}

	PayloadBase64 := common.Base64Encode(PayloadJSON)
	hmac := common.GetHMAC(common.HashSHA512_384, []byte(PayloadBase64), []byte(b.APISecret))
	headers := make(map[string]string)
	headers["X-BFX-APIKEY"] = b.APIKey
	headers["X-BFX-PAYLOAD"] = PayloadBase64
	headers["X-BFX-SIGNATURE"] = common.HexEncodeToString(hmac)

	resp, err := common.SendHTTPRequest(
		method, bitfinexAPIURL+path, headers, strings.NewReader(""),
	)
	if err != nil {
		return err
	}

	if b.Verbose {
		log.Printf("Received raw: \n%s\n", resp)
	}

	if err = common.JSONDecode([]byte(resp), &respErr); err == nil {
		if len(respErr.Message) != 0 {
			return errors.New("Responded Error Issue: " + respErr.Message)
		}
	}

	if err = common.JSONDecode([]byte(resp), &result); err != nil {
		return errors.New("sendAuthenticatedHTTPRequest: Unable to JSON Unmarshal response")
	}
	return nil
}

// SendRateLimitedHTTPRequest sends an HTTP request if the given number of requests per minute
// hasn't been exceeded for the specified method & path and unmarshals the response into the
// result parameter. If the number of requests per minute has been exceeded this method will
// set the result to the default value (which can be a pointer, but must not be nil).
func (b *Bitfinex) SendRateLimitedHTTPRequest(requestsPerMin uint, method, path string, params map[string]interface{},
	result interface{}, defaultValue interface{}) error {
	rateLimit := b.rateLimits[method+path]
	if rateLimit == nil {
		rateLimit = &rateLimitInfo{}
		b.rateLimits[method+path] = rateLimit
	}

	curTimeStamp := time.Now().Unix()
	if (rateLimit.StartTime == 0) || ((curTimeStamp - rateLimit.StartTime) > 90) {
		rateLimit.RequestCount = 0
		rateLimit.StartTime = curTimeStamp
	}
	if rateLimit.RequestCount < requestsPerMin {
		rateLimit.RequestCount++
	} else {
		// set result to default value
		rv := reflect.ValueOf(result)
		if rv.Kind() != reflect.Ptr || rv.IsNil() {
			return errors.New("result must be a non-nil pointer")
		}
		dv := reflect.ValueOf(defaultValue)
		if !dv.IsValid() {
			return errors.New("default value must be not be nil")
		}
		if dv.Kind() == reflect.Ptr {
			reflect.Indirect(rv).Set(dv.Elem())
		} else {
			reflect.Indirect(rv).Set(dv)
		}
		return nil
	}

	return b.SendAuthenticatedHTTPRequest(method, path, params, result)
}
