package poloniex

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strconv"
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
	POLONIEX_API_URL                = "https://poloniex.com"
	POLONIEX_API_TRADING_ENDPOINT   = "tradingApi"
	POLONIEX_API_VERSION            = "1"
	POLONIEX_BALANCES               = "returnBalances"
	POLONIEX_BALANCES_COMPLETE      = "returnCompleteBalances"
	POLONIEX_DEPOSIT_ADDRESSES      = "returnDepositAddresses"
	POLONIEX_GENERATE_NEW_ADDRESS   = "generateNewAddress"
	POLONIEX_DEPOSITS_WITHDRAWALS   = "returnDepositsWithdrawals"
	POLONIEX_ORDERS                 = "returnOpenOrders"
	POLONIEX_TRADE_HISTORY          = "returnTradeHistory"
	POLONIEX_ORDER_TRADES           = "returnOrderTrades"
	POLONIEX_ORDER_BUY              = "buy"
	POLONIEX_ORDER_SELL             = "sell"
	POLONIEX_ORDER_CANCEL           = "cancelOrder"
	POLONIEX_ORDER_MOVE             = "moveOrder"
	POLONIEX_WITHDRAW               = "withdraw"
	POLONIEX_FEE_INFO               = "returnFeeInfo"
	POLONIEX_AVAILABLE_BALANCES     = "returnAvailableAccountBalances"
	POLONIEX_TRADABLE_BALANCES      = "returnTradableBalances"
	POLONIEX_TRANSFER_BALANCE       = "transferBalance"
	POLONIEX_MARGIN_ACCOUNT_SUMMARY = "returnMarginAccountSummary"
	POLONIEX_MARGIN_BUY             = "marginBuy"
	POLONIEX_MARGIN_SELL            = "marginSell"
	POLONIEX_MARGIN_POSITION        = "getMarginPosition"
	POLONIEX_MARGIN_POSITION_CLOSE  = "closeMarginPosition"
	POLONIEX_CREATE_LOAN_OFFER      = "createLoanOffer"
	POLONIEX_CANCEL_LOAN_OFFER      = "cancelLoanOffer"
	POLONIEX_OPEN_LOAN_OFFERS       = "returnOpenLoanOffers"
	POLONIEX_ACTIVE_LOANS           = "returnActiveLoans"
	POLONIEX_LENDING_HISTORY        = "returnLendingHistory"
	POLONIEX_AUTO_RENEW             = "toggleAutoRenew"
)

type Poloniex struct {
	exchange.Base
	currencyPairs map[pair.CurrencyItem]*exchange.CurrencyPairInfo
}

func (p *Poloniex) SetDefaults() {
	p.Name = "Poloniex"
	p.Enabled = false
	p.Fee = 0
	p.Verbose = false
	p.Websocket = false
	p.RESTPollingDelay = 10
	p.RequestCurrencyPairFormat.Delimiter = "_"
	p.RequestCurrencyPairFormat.Uppercase = true
	p.ConfigCurrencyPairFormat.Delimiter = "_"
	p.ConfigCurrencyPairFormat.Uppercase = true
	p.AssetTypes = []string{ticker.Spot}
	p.Orderbooks = orderbook.Init()
}

func (p *Poloniex) Setup(exch config.ExchangeConfig) {
	if !exch.Enabled {
		p.SetEnabled(false)
	} else {
		p.Enabled = true
		p.AuthenticatedAPISupport = exch.AuthenticatedAPISupport
		p.SetAPIKeys(exch.APIKey, exch.APISecret, "", false)
		p.RESTPollingDelay = exch.RESTPollingDelay
		p.Verbose = exch.Verbose
		p.Websocket = exch.Websocket

		p.Base.CommonSetup(exch)
		err := p.SetCurrencyPairFormat()
		if err != nil {
			log.Fatal(err)
		}
		err = p.SetAssetTypes()
		if err != nil {
			log.Fatal(err)
		}
	}
}

// CurrencyPairToSymbol converts a currency pair to a symbol (exchange specific market identifier).
func (p *Poloniex) CurrencyPairToSymbol(cp pair.CurrencyPair) string {
	return cp.
		// Poloniex symbols are inverted currency pairs
		Invert().
		Display(p.RequestCurrencyPairFormat.Delimiter, p.RequestCurrencyPairFormat.Uppercase).
		String()
}

// SymbolToCurrencyPair converts a symbol (exchange specific market identifier) to a currency pair.
func (p *Poloniex) SymbolToCurrencyPair(symbol string) pair.CurrencyPair {
	cp := pair.NewCurrencyPairDelimiter(symbol, p.RequestCurrencyPairFormat.Delimiter)
	// Poloniex symbols are inverted currency pairs, so invert them here to get a proper currency pair
	return cp.Invert()
}

// GetLimits returns price/amount limits for the exchange.
func (p *Poloniex) GetLimits() exchange.ILimits {
	return &exchange.DefaultExchangeLimits{}
}

// Returns currency pairs that can be used by the exchange account associated with this bot.
// Use FormatExchangeCurrency to get the right key.
func (p *Poloniex) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	return p.currencyPairs
}

func (p *Poloniex) GetFee() float64 {
	return p.Fee
}

func (p *Poloniex) GetTicker() (map[string]PoloniexTicker, error) {
	type response struct {
		Data map[string]PoloniexTicker
	}

	resp := response{}
	path := fmt.Sprintf("%s/public?command=returnTicker", POLONIEX_API_URL)
	err := common.SendHTTPGetRequest(path, true, p.Verbose, &resp.Data)

	if err != nil {
		return resp.Data, err
	}
	return resp.Data, nil
}

func (p *Poloniex) GetVolume() (interface{}, error) {
	var resp interface{}
	path := fmt.Sprintf("%s/public?command=return24hVolume", POLONIEX_API_URL)
	err := common.SendHTTPGetRequest(path, true, p.Verbose, &resp)

	if err != nil {
		return resp, err
	}
	return resp, nil
}

func (p *Poloniex) GetOrderbook(currencyPair string, depth int) (PoloniexOrderbook, error) {
	vals := url.Values{}
	vals.Set("currencyPair", currencyPair)

	if depth != 0 {
		vals.Set("depth", strconv.Itoa(depth))
	}

	resp := PoloniexOrderbookResponse{}
	path := fmt.Sprintf("%s/public?command=returnOrderBook&%s", POLONIEX_API_URL, vals.Encode())
	err := common.SendHTTPGetRequest(path, true, p.Verbose, &resp)

	if err != nil {
		return PoloniexOrderbook{}, err
	}

	ob := PoloniexOrderbook{}
	for x := range resp.Asks {
		data := resp.Asks[x]
		price, err := strconv.ParseFloat(data[0].(string), 64)
		if err != nil {
			return ob, err
		}
		amount := data[1].(float64)
		ob.Asks = append(ob.Asks, PoloniexOrderbookItem{Price: price, Amount: amount})
	}

	for x := range resp.Bids {
		data := resp.Bids[x]
		price, err := strconv.ParseFloat(data[0].(string), 64)
		if err != nil {
			return ob, err
		}
		amount := data[1].(float64)
		ob.Bids = append(ob.Bids, PoloniexOrderbookItem{Price: price, Amount: amount})
	}
	return ob, nil
}

func (p *Poloniex) GetTradeHistory(currencyPair, start, end string) ([]PoloniexTradeHistory, error) {
	vals := url.Values{}
	vals.Set("currencyPair", currencyPair)

	if start != "" {
		vals.Set("start", start)
	}

	if end != "" {
		vals.Set("end", end)
	}

	resp := []PoloniexTradeHistory{}
	path := fmt.Sprintf("%s/public?command=returnTradeHistory&%s", POLONIEX_API_URL, vals.Encode())
	err := common.SendHTTPGetRequest(path, true, p.Verbose, &resp)

	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *Poloniex) GetChartData(currencyPair, start, end, period string) ([]PoloniexChartData, error) {
	vals := url.Values{}
	vals.Set("currencyPair", currencyPair)

	if start != "" {
		vals.Set("start", start)
	}

	if end != "" {
		vals.Set("end", end)
	}

	if period != "" {
		vals.Set("period", period)
	}

	resp := []PoloniexChartData{}
	path := fmt.Sprintf("%s/public?command=returnChartData&%s", POLONIEX_API_URL, vals.Encode())
	err := common.SendHTTPGetRequest(path, true, p.Verbose, &resp)

	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *Poloniex) GetCurrencies() (map[string]PoloniexCurrencies, error) {
	type Response struct {
		Data map[string]PoloniexCurrencies
	}
	resp := Response{}
	path := fmt.Sprintf("%s/public?command=returnCurrencies", POLONIEX_API_URL)
	err := common.SendHTTPGetRequest(path, true, p.Verbose, &resp.Data)

	if err != nil {
		return resp.Data, err
	}
	return resp.Data, nil
}

func (p *Poloniex) GetLoanOrders(currency string) (PoloniexLoanOrders, error) {
	resp := PoloniexLoanOrders{}
	path := fmt.Sprintf("%s/public?command=returnLoanOrders&currency=%s", POLONIEX_API_URL, currency)
	err := common.SendHTTPGetRequest(path, true, p.Verbose, &resp)

	if err != nil {
		return resp, err
	}
	return resp, nil
}

func (p *Poloniex) GetBalances() (PoloniexBalance, error) {
	var result interface{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_BALANCES, url.Values{}, &result)

	if err != nil {
		return PoloniexBalance{}, err
	}

	data := result.(map[string]interface{})
	balance := PoloniexBalance{}
	balance.Currency = make(map[string]float64)

	for x, y := range data {
		balance.Currency[x], _ = strconv.ParseFloat(y.(string), 64)
	}

	return balance, nil
}

type PoloniexCompleteBalances struct {
	Currency map[string]PoloniexCompleteBalance
}

func (p *Poloniex) GetCompleteBalances() (PoloniexCompleteBalances, error) {
	var result interface{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_BALANCES_COMPLETE, url.Values{}, &result)

	if err != nil {
		return PoloniexCompleteBalances{}, err
	}

	data := result.(map[string]interface{})
	balance := PoloniexCompleteBalances{}
	balance.Currency = make(map[string]PoloniexCompleteBalance)

	for x, y := range data {
		dataVals := y.(map[string]interface{})
		balancesData := PoloniexCompleteBalance{}
		balancesData.Available, _ = strconv.ParseFloat(dataVals["available"].(string), 64)
		balancesData.OnOrders, _ = strconv.ParseFloat(dataVals["onOrders"].(string), 64)
		balancesData.BTCValue, _ = strconv.ParseFloat(dataVals["btcValue"].(string), 64)
		balance.Currency[x] = balancesData
	}

	return balance, nil
}

func (p *Poloniex) GetDepositAddresses() (PoloniexDepositAddresses, error) {
	var result interface{}
	addresses := PoloniexDepositAddresses{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_DEPOSIT_ADDRESSES, url.Values{}, &result)

	if err != nil {
		return addresses, err
	}

	addresses.Addresses = make(map[string]string)
	data := result.(map[string]interface{})
	for x, y := range data {
		addresses.Addresses[x] = y.(string)
	}

	return addresses, nil
}

func (p *Poloniex) GenerateNewAddress(currency string) (string, error) {
	type Response struct {
		Success  int
		Error    string
		Response string
	}
	resp := Response{}
	values := url.Values{}
	values.Set("currency", currency)

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_GENERATE_NEW_ADDRESS, values, &resp)

	if err != nil {
		return "", err
	}

	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}

	return resp.Response, nil
}

func (p *Poloniex) GetDepositsWithdrawals(start, end string) (PoloniexDepositsWithdrawals, error) {
	resp := PoloniexDepositsWithdrawals{}
	values := url.Values{}

	if start != "" {
		values.Set("start", start)
	} else {
		values.Set("start", "0")
	}

	if end != "" {
		values.Set("end", end)
	} else {
		values.Set("end", strconv.FormatInt(time.Now().Unix(), 10))
	}

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_DEPOSITS_WITHDRAWALS, values, &resp)

	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (p *Poloniex) GetOpenOrders(currency string) (PoloniexOpenOrdersResponse, error) {
	values := url.Values{}

	values.Set("currencyPair", currency)
	result := PoloniexOpenOrdersResponse{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_ORDERS, values, &result.Data)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) GetAllOpenOrders() (PoloniexOpenOrdersResponseAll, error) {
	values := url.Values{}

	values.Set("currencyPair", "all")
	result := PoloniexOpenOrdersResponseAll{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_ORDERS, values, &result.Data)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) GetAuthenticatedTradeHistory(currency, start, end string) (interface{}, error) {
	values := url.Values{}

	if start != "" {
		values.Set("start", start)
	}

	if end != "" {
		values.Set("end", end)
	}

	if currency != "" && currency != "all" {
		values.Set("currencyPair", currency)
		result := PoloniexAuthenticatedTradeHistoryResponse{}
		err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_TRADE_HISTORY, values, &result.Data)

		if err != nil {
			return result, err
		}

		return result, nil
	} else {
		values.Set("currencyPair", "all")
		result := PoloniexAuthenticatedTradeHistoryAll{}
		err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_TRADE_HISTORY, values, &result.Data)

		if err != nil {
			return result, err
		}

		return result, nil
	}
}

func (p *Poloniex) GetOrderTrades(orderID string) (PoloniexAuthentictedOrderTradesResponse, error) {
	result := PoloniexAuthentictedOrderTradesResponse{}
	values := url.Values{}
	values.Set("orderNumber", orderID)
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_ORDER_TRADES, values, &result.Data)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) PlaceOrder(currency string, rate, amount float64, immediate, fillOrKill bool, orderType exchange.OrderSide) (PoloniexOrderResponse, error) {
	result := PoloniexOrderResponse{}
	values := url.Values{}

	values.Set("currencyPair", currency)
	values.Set("rate", strconv.FormatFloat(rate, 'f', -1, 64))
	values.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))

	if immediate {
		values.Set("immediateOrCancel", "1")
	}

	if fillOrKill {
		values.Set("fillOrKill", "1")
	}

	err := p.SendAuthenticatedHTTPRequest("POST", string(orderType), values, &result)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) GetOrder(orderID string) (*exchange.Order, error) {
	response, err := p.GetOrderTrades(orderID)
	// TODO: figure out what kind of errors are returned when the order isn't found vs there are
	// no trades for the order... the former error we should propagate to the caller, the latter
	// should be swallowed.
	if err != nil {
		return nil, err
	}

	var currencyPair pair.CurrencyPair
	var side exchange.OrderSide
	rateSum := decimal.Zero
	filledAmount := decimal.Zero
	for i, trade := range response.Data {
		if i == 0 {
			currencyPair = p.SymbolToCurrencyPair(trade.CurrencyPair)
			rateSum = rateSum.Add(decimal.NewFromFloat(trade.Rate))
			side = exchange.OrderSide(trade.Type)
		}
		filledAmount = filledAmount.Add(decimal.NewFromFloat(trade.Total))
	}

	var avgRate float64
	numTrades := len(response.Data)
	if numTrades > 0 {
		avgRate, _ = rateSum.Div(decimal.New(int64(numTrades), 0)).Float64()
	}
	orderFilledAmount, _ := filledAmount.Float64()
	order := &exchange.Order{
		CurrencyPair: currencyPair,
		Side:         side,
		FilledAmount: orderFilledAmount,
		Rate:         avgRate,
		// The order could be filled in full or cancelled, if it's cancelled it could be partly
		// filled or not at all. There isn't enough info to tell for sure!
		Status:  exchange.OrderStatusAborted,
		OrderID: orderID,
	}
	return order, nil
}

func (p *Poloniex) convertOrderToExchangeOrder(order *PoloniexOrder, symbol string) *exchange.Order {
	retOrder := &exchange.Order{}
	retOrder.OrderID = strconv.FormatInt(order.OrderNumber, 10)

	//All orders that get returned are active
	//TODO how to handle canceled orders
	retOrder.Status = exchange.OrderStatusActive

	//TODO: verify total is actually correct, its the total filled??
	retOrder.FilledAmount = order.Total
	retOrder.RemainingAmount = order.Amount - order.Total
	retOrder.Amount = order.Amount
	retOrder.Rate = order.Rate
	retOrder.CreatedAt = order.Date.Unix()
	retOrder.CurrencyPair = p.SymbolToCurrencyPair(symbol)
	retOrder.Side = exchange.OrderSide(order.Type) //no conversion neccessary this exchange uses the word buy/sell

	return retOrder
}

func (p *Poloniex) GetOrders() ([]*exchange.Order, error) {
	ret := []*exchange.Order{}

	activeorders, err := p.GetAllOpenOrders()
	if err != nil {
		return ret, err
	}

	for symbol, orders := range activeorders.Data {
		for _, order := range orders {
			retOrder := p.convertOrderToExchangeOrder(order, symbol)
			ret = append(ret, retOrder)
		}
	}
	return ret, nil
}

func (p *Poloniex) NewOrder(
	currencyPair pair.CurrencyPair, amount, price float64, side exchange.OrderSide,
	orderType exchange.OrderType) (string, error) {
	/*
		You may optionally set "fillOrKill", "immediateOrCancel", "postOnly".
		- A fill-or-kill order will either fill in its entirety or be completely aborted.
		- An immediate-or-cancel order can be partially or completely filled,
		  but any portion of the order that cannot be filled immediately will be canceled
		  rather than left on the order book.
		- A post-only order will only be placed if no portion of it fills immediately;
		  this guarantees you will never pay the taker fee on any part of the order that fills.
	*/
	// For now just support plain limit orders.
	immediate := false
	fillOrKill := false

	symbol := p.CurrencyPairToSymbol(currencyPair)
	response, err := p.PlaceOrder(symbol, price, amount, immediate, fillOrKill, side)

	if err != nil {
		return "", err
	}
	orderID := strconv.FormatInt(response.OrderNumber, 10)
	//TODO returns a list of finished trades PoloniexResultingTrades
	//guessing this exchange can fill automattically???

	return orderID, nil
}

func (p *Poloniex) CancelOrder(orderstr string) error {
	var err error
	var orderID int64
	if orderID, err = strconv.ParseInt(orderstr, 10, 64); err == nil {
		return err
	}
	_, err = p.cancelOrder(orderID)
	return err
}

func (p *Poloniex) cancelOrder(orderID int64) (bool, error) {
	result := PoloniexGenericResponse{}
	values := url.Values{}
	values.Set("orderNumber", strconv.FormatInt(orderID, 10))

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_ORDER_CANCEL, values, &result)

	if err != nil {
		return false, err
	}

	if result.Success != 1 {
		return false, errors.New(result.Error)
	}

	return true, nil
}

func (p *Poloniex) MoveOrder(orderID int64, rate, amount float64) (PoloniexMoveOrderResponse, error) {
	result := PoloniexMoveOrderResponse{}
	values := url.Values{}
	values.Set("orderNumber", strconv.FormatInt(orderID, 10))
	values.Set("rate", strconv.FormatFloat(rate, 'f', -1, 64))

	if amount != 0 {
		values.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	}

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_ORDER_MOVE, values, &result)

	if err != nil {
		return result, err
	}

	if result.Success != 1 {
		return result, errors.New(result.Error)
	}

	return result, nil
}

func (p *Poloniex) Withdraw(currency, address string, amount float64) (bool, error) {
	result := PoloniexWithdraw{}
	values := url.Values{}

	values.Set("currency", currency)
	values.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	values.Set("address", address)

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_WITHDRAW, values, &result)

	if err != nil {
		return false, err
	}

	if result.Error != "" {
		return false, errors.New(result.Error)
	}

	return true, nil
}

func (p *Poloniex) GetFeeInfo() (PoloniexFee, error) {
	result := PoloniexFee{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_FEE_INFO, url.Values{}, &result)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) GetTradableBalances() (map[string]map[string]float64, error) {
	type Response struct {
		Data map[string]map[string]interface{}
	}
	result := Response{}

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_TRADABLE_BALANCES, url.Values{}, &result.Data)

	if err != nil {
		return nil, err
	}

	balances := make(map[string]map[string]float64)

	for x, y := range result.Data {
		balances[x] = make(map[string]float64)
		for z, w := range y {
			balances[x][z], _ = strconv.ParseFloat(w.(string), 64)
		}
	}

	return balances, nil
}

func (p *Poloniex) TransferBalance(currency, from, to string, amount float64) (bool, error) {
	values := url.Values{}
	result := PoloniexGenericResponse{}

	values.Set("currency", currency)
	values.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	values.Set("fromAccount", from)
	values.Set("toAccount", to)

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_TRANSFER_BALANCE, values, &result)

	if err != nil {
		return false, err
	}

	if result.Error != "" && result.Success != 1 {
		return false, errors.New(result.Error)
	}

	return true, nil
}

func (p *Poloniex) GetMarginAccountSummary() (PoloniexMargin, error) {
	result := PoloniexMargin{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_MARGIN_ACCOUNT_SUMMARY, url.Values{}, &result)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) PlaceMarginOrder(currency string, rate, amount, lendingRate float64, buy bool) (PoloniexOrderResponse, error) {
	result := PoloniexOrderResponse{}
	values := url.Values{}

	var orderType string
	if buy {
		orderType = POLONIEX_MARGIN_BUY
	} else {
		orderType = POLONIEX_MARGIN_SELL
	}

	values.Set("currencyPair", currency)
	values.Set("rate", strconv.FormatFloat(rate, 'f', -1, 64))
	values.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))

	if lendingRate != 0 {
		values.Set("lendingRate", strconv.FormatFloat(lendingRate, 'f', -1, 64))
	}

	err := p.SendAuthenticatedHTTPRequest("POST", orderType, values, &result)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) GetMarginPosition(currency string) (interface{}, error) {
	values := url.Values{}

	if currency != "" && currency != "all" {
		values.Set("currencyPair", currency)
		result := PoloniexMarginPosition{}
		err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_MARGIN_POSITION, values, &result)

		if err != nil {
			return result, err
		}

		return result, nil
	} else {
		values.Set("currencyPair", "all")

		type Response struct {
			Data map[string]PoloniexMarginPosition
		}

		result := Response{}
		err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_MARGIN_POSITION, values, &result.Data)

		if err != nil {
			return result, err
		}

		return result, nil
	}
}

func (p *Poloniex) CloseMarginPosition(currency string) (bool, error) {
	values := url.Values{}
	values.Set("currencyPair", currency)
	result := PoloniexGenericResponse{}

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_MARGIN_POSITION_CLOSE, values, &result)

	if err != nil {
		return false, err
	}

	if result.Success == 0 {
		return false, errors.New(result.Error)
	}

	return true, nil
}

func (p *Poloniex) CreateLoanOffer(currency string, amount, rate float64, duration int, autoRenew bool) (int64, error) {
	values := url.Values{}
	values.Set("currency", currency)
	values.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	values.Set("duration", strconv.Itoa(duration))

	if autoRenew {
		values.Set("autoRenew", "1")
	} else {
		values.Set("autoRenew", "0")
	}

	values.Set("lendingRate", strconv.FormatFloat(rate, 'f', -1, 64))

	type Response struct {
		Success int    `json:"success"`
		Error   string `json:"error"`
		OrderID int64  `json:"orderID"`
	}

	result := Response{}

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_CREATE_LOAN_OFFER, values, &result)

	if err != nil {
		return 0, err
	}

	if result.Success == 0 {
		return 0, errors.New(result.Error)
	}

	return result.OrderID, nil
}

func (p *Poloniex) CancelLoanOffer(orderNumber int64) (bool, error) {
	result := PoloniexGenericResponse{}
	values := url.Values{}
	values.Set("orderID", strconv.FormatInt(orderNumber, 10))

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_CANCEL_LOAN_OFFER, values, &result)

	if err != nil {
		return false, err
	}

	if result.Success == 0 {
		return false, errors.New(result.Error)
	}

	return true, nil
}

func (p *Poloniex) GetOpenLoanOffers() (map[string][]PoloniexLoanOffer, error) {
	type Response struct {
		Data map[string][]PoloniexLoanOffer
	}
	result := Response{}

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_OPEN_LOAN_OFFERS, url.Values{}, &result.Data)

	if err != nil {
		return nil, err
	}

	if result.Data == nil {
		return nil, errors.New("There are no open loan offers.")
	}

	return result.Data, nil
}

func (p *Poloniex) GetActiveLoans() (PoloniexActiveLoans, error) {
	result := PoloniexActiveLoans{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_ACTIVE_LOANS, url.Values{}, &result)

	if err != nil {
		return result, err
	}

	return result, nil
}

func (p *Poloniex) GetLendingHistory(start, end string) ([]PoloniexLendingHistory, error) {
	vals := url.Values{}

	if start != "" {
		vals.Set("start", start)
	}

	if end != "" {
		vals.Set("end", end)
	}

	resp := []PoloniexLendingHistory{}
	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_LENDING_HISTORY, vals, &resp)

	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *Poloniex) ToggleAutoRenew(orderNumber int64) (bool, error) {
	values := url.Values{}
	values.Set("orderNumber", strconv.FormatInt(orderNumber, 10))
	result := PoloniexGenericResponse{}

	err := p.SendAuthenticatedHTTPRequest("POST", POLONIEX_AUTO_RENEW, values, &result)

	if err != nil {
		return false, err
	}

	if result.Success == 0 {
		return false, errors.New(result.Error)
	}

	return true, nil
}

func (p *Poloniex) SendAuthenticatedHTTPRequest(method, endpoint string, values url.Values, result interface{}) error {
	if !p.AuthenticatedAPISupport {
		return fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, p.Name)
	}
	headers := make(map[string]string)
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	headers["Key"] = p.APIKey

	if p.Nonce.Get() == 0 {
		p.Nonce.Set(time.Now().UnixNano())
	} else {
		p.Nonce.Inc()
	}
	values.Set("nonce", p.Nonce.String())
	values.Set("command", endpoint)

	hmac := common.GetHMAC(common.HashSHA512, []byte(values.Encode()), []byte(p.APISecret))
	headers["Sign"] = common.HexEncodeToString(hmac)

	path := fmt.Sprintf("%s/%s", POLONIEX_API_URL, POLONIEX_API_TRADING_ENDPOINT)
	resp, err := common.SendHTTPRequest(method, path, headers, bytes.NewBufferString(values.Encode()))

	if err != nil {
		return err
	}

	err = common.JSONDecode([]byte(resp), &result)

	if err != nil {
		return errors.New("Unable to JSON Unmarshal response.")
	}
	return nil
}
