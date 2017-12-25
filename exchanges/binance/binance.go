package binance

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	exchange "github.com/mattkanwisher/cryptofiend/exchanges"
)

const (
	binanceBaseURL          = "https://www.binance.com/"
	binanceExchangeInfoPath = "api/v1/exchangeInfo"
	binanceAccountPath      = "api/v3/account"
	binanceOpenOrdersPath   = "api/v3/openOrders"
	binanceOrderPath        = "api/v3/order"
	binanceOrderTestPath    = "api/v3/order/test"
	binanceDepthPath        = "api/v1/depth"
)

// BinanceErrCode enum represents a frequently encountered subset of the error codes documented at:
// https://github.com/binance-exchange/binance-official-api-docs/blob/master/errors.md
type BinanceErrCode int32

const (
	TooManyRequestsErrCode  BinanceErrCode = -1003
	InvalidTimestampErrCode BinanceErrCode = -1021 // fix: sync your computer clock to internet time
)

type Binance struct {
	exchange.Base
	// Maps HTTP method & path to a timestamp (in msecs) of the last time a request was sent
	rateLimits map[string]int64
	// Timestamp (in msecs) of the last time the Binance server rate limited a request
	ipBanStartTime int64
	// Maps symbol (exchange specific market identifier) to currency pair info
	currencyPairs    map[pair.CurrencyItem]*exchange.CurrencyPairInfo
	symbolDetailsMap map[pair.CurrencyItem]*symbolDetails
	// Cached data that's returned when HTTP requests are rate-limited
	lastAccountInfo AccountInfo
	lastOpenOrders  map[string][]Order
	lastMarketData  map[string]*MarketData
}

// CurrencyPairToSymbol converts a currency pair to a symbol (exchange specific market identifier).
func (b *Binance) CurrencyPairToSymbol(p pair.CurrencyPair) string {
	return p.
		Display(b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase).
		String()
}

// SymbolToCurrencyPair converts a symbol (exchange specific market identifier) to a currency pair.
func (b *Binance) SymbolToCurrencyPair(symbol string) (pair.CurrencyPair, error) {
	if p, exists := b.currencyPairs[pair.CurrencyItem(symbol)]; exists {
		return p.Currency.FormatPair(
			b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase), nil
	}
	return pair.CurrencyPair{}, fmt.Errorf("no currency pair found for '%s' symbol", symbol)
}

// FetchExchangeInfo fetches current exchange trading rules and symbol information.
func (b *Binance) FetchExchangeInfo() (*ExchangeInfo, error) {
	response := ExchangeInfo{}
	err := common.SendHTTPGetRequest(binanceBaseURL+binanceExchangeInfoPath, true, b.Verbose, &response)
	return &response, err
}

// FetchAccountInfo fetches current account information.
// If this method gets rate limited it will return the account info obtained during the
// last successful fetch, and an error matching exchange.WarningHTTPRequestRateLimited.
func (b *Binance) FetchAccountInfo() (*AccountInfo, error) {
	response := AccountInfo{}
	err := b.SendRateLimitedHTTPRequest(20, http.MethodGet, binanceAccountPath, nil,
		RequestSecuritySign, &response, b.lastAccountInfo)
	if err != nil {
		return &response, err
	}
	b.lastAccountInfo = response
	return &response, nil
}

// FetchOpenOrders fetches all currently open orders.
// If the symbol parameter is blank all open orders for the account will be returned,
// this should generally be avoided as it's an expensive operation that can very quickly put
// you over the request rate limit if this method is called multiple times per minute.
// If this method gets rate limited it will return the set of orders obtained during the
// last successful fetch, and an error matching exchange.WarningHTTPRequestRateLimited.
func (b *Binance) FetchOpenOrders(symbol string) ([]Order, error) {
	v := url.Values{}
	if symbol != "" {
		v.Set("symbol", symbol)
	}
	lastOpenOrders := b.lastOpenOrders[symbol]
	if lastOpenOrders == nil {
		lastOpenOrders = []Order{}
	}
	response := []Order{}
	err := b.SendRateLimitedHTTPRequest(10, http.MethodGet, binanceOpenOrdersPath, v,
		RequestSecuritySign, &response, lastOpenOrders)
	if err != nil {
		return response, err
	}
	b.lastOpenOrders[symbol] = response
	return response, nil
}

type PostOrderParams struct {
	Symbol           string
	Side             OrderSide
	Type             OrderType
	TimeInForce      TimeInForce
	Quantity         float64
	Price            float64
	NewClientOrderID string
	StopPrice        float64
	IcebergQty       float64
	// Set to true to submit the order to the test endpoint for validation,
	// it won't be sent to the exchange matching engine.
	ValidateOnly bool
}

func (b *Binance) PostOrderAck(params *PostOrderParams) (*PostOrderAckResponse, error) {
	v := url.Values{}
	v.Set("symbol", params.Symbol)
	v.Set("side", string(params.Side))
	v.Set("type", string(params.Type))
	v.Set("timeInForce", string(params.TimeInForce))
	v.Set("quantity", strconv.FormatFloat(params.Quantity, 'f', -1, 64))
	v.Set("price", strconv.FormatFloat(params.Price, 'f', -1, 64))
	if params.NewClientOrderID != "" {
		v.Set("newClientOrderId", params.NewClientOrderID)
	}
	if params.StopPrice != 0 {
		v.Set("stopPrice", strconv.FormatFloat(params.StopPrice, 'f', -1, 64))
	}
	if params.IcebergQty != 0 {
		v.Set("icebergQty", strconv.FormatFloat(params.IcebergQty, 'f', -1, 64))
	}
	v.Set("newOrderRespType", "ACK")

	response := PostOrderAckResponse{}
	path := binanceOrderPath
	if params.ValidateOnly {
		path = binanceOrderTestPath
	}
	_, err := b.SendHTTPRequest(http.MethodPost, path, v, RequestSecuritySign, &response)
	return &response, err
}

// FetchOrder fetches an order from the exchange, either orderID or clientOrderID must be provided.
func (b *Binance) FetchOrder(symbol string, orderID int64, clientOrderID string) (*Order, error) {
	v := url.Values{}
	v.Set("symbol", symbol)
	if orderID != 0 {
		v.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if clientOrderID != "" {
		v.Set("origClientOrderId", clientOrderID)
	}
	response := Order{}
	_, err := b.SendHTTPRequest(http.MethodGet, binanceOrderPath, v, RequestSecuritySign, &response)
	return &response, err
}

// DeleteOrder cancels an active order on the exchange, either orderID or clientOrderID must be provided.
func (b *Binance) DeleteOrder(symbol string, orderID int64, clientOrderID string) error {
	v := url.Values{}
	v.Set("symbol", symbol)
	if orderID != 0 {
		v.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if clientOrderID != "" {
		v.Set("origClientOrderId", clientOrderID)
	}
	response := DeleteOrderResponse{}
	_, err := b.SendHTTPRequest(http.MethodDelete, binanceOrderPath, v, RequestSecuritySign, &response)
	return err
}

// FetchMarketData fetches the orderbooks for the given symbol.
// The limit parameter can be -1, 0, 5, 10, 20, 50, 100, 200, 1000.
// Set the limit to -1 to use the default value (currently 100), or to 0 to disable the limit
// (this can return a lot of data, so should avoided).
// NOTE: Unlike most other exchange Binance requires a valid API key when fetching market data.
// If this method gets rate limited it will return the market data obtained during the
// last successful fetch, and an error matching exchange.WarningHTTPRequestRateLimited.
func (b *Binance) FetchMarketData(symbol string, limit int64) (*MarketData, error) {
	v := url.Values{}
	v.Set("symbol", symbol)
	if limit > -1 {
		v.Set("limit", strconv.FormatInt(limit, 10))
	}

	lastMarketData := b.lastMarketData[symbol]
	if lastMarketData == nil {
		lastMarketData = &MarketData{}
	}
	response := MarketData{}
	err := b.SendRateLimitedHTTPRequest(20, http.MethodGet, binanceDepthPath, v, RequestSecurityAuth,
		&response, lastMarketData)
	b.lastMarketData[symbol] = &response
	return &response, err
}

type RequestSecurityEnum uint8

const (
	// Don't send API key
	RequestSecurityNone RequestSecurityEnum = iota
	// Only send API key
	RequestSecurityAuth
	// Send API key & sign
	RequestSecuritySign
)

// SendAuthenticatedHTTPRequest sends a POST request to an authenticated endpoint, the response is
// decoded into the result object.
// Returns the Binance error code and error message (if any).
func (b *Binance) SendHTTPRequest(method, path string, params url.Values, security RequestSecurityEnum,
	result interface{}) (int, error) {
	if (security != RequestSecurityNone) && !b.AuthenticatedAPISupport {
		return 0, fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, b.Name)
	}

	if b.Verbose {
		log.Printf("Request params: %v\n", params)
	}

	headers := make(http.Header)
	headers["Accept"] = []string{"application/json"}

	var payload string
	if len(params) > 0 {
		payload = params.Encode()
	}

	if security == RequestSecuritySign {
		recvWindow := 5000
		// HACK: Subtract 1 sec from the real timestamp to get around incessant timestamp errors
		// from Binance.
		timestamp := time.Now().UnixNano()/(1000*1000) - 1000 // must be in milliseconds
		timeWindow := fmt.Sprintf("timestamp=%v&recvWindow=%d", timestamp, recvWindow)
		if payload != "" {
			payload += "&" + timeWindow
		} else {
			payload = timeWindow
		}
		hmac := common.GetHMAC(common.HashSHA256, []byte(payload), []byte(b.APISecret))
		payload = fmt.Sprintf("%s&signature=%s", payload, hex.EncodeToString(hmac))
	}

	if security != RequestSecurityNone {
		headers["X-MBX-APIKEY"] = []string{b.APIKey}
	}

	var resp string
	var statusCode int
	var err error
	if method == http.MethodGet {
		resp, statusCode, err = common.SendHTTPRequest2(
			method, fmt.Sprintf("%s%s?%s", binanceBaseURL, path, payload), headers, nil)
	} else {
		headers["Content-Type"] = []string{"application/x-www-form-urlencoded"}
		resp, statusCode, err = common.SendHTTPRequest2(method,
			binanceBaseURL+path, headers, strings.NewReader(payload))
	}

	if err != nil {
		return 0, err
	}

	if b.Verbose {
		log.Printf("Received raw: \n%s\n", resp)
	}

	if 200 <= statusCode && statusCode <= 299 {
		if err = common.JSONDecode([]byte(resp), &result); err != nil {
			return statusCode, errors.New("failed to unmarshal response")
		}
	} else {
		var errInfo ErrorInfo
		if err = common.JSONDecode([]byte(resp), &errInfo); err != nil {
			return 0, errors.New("failed to unmarshal error info")
		}
		return int(errInfo.Code), errors.New(errInfo.Message)
	}

	return 0, nil
}

// SendRateLimitedHTTPRequest sends an HTTP request if the given number of requests per minute
// hasn't been exceeded for the specified method & path and unmarshals the response into the
// result parameter. If the number of requests per minute has been exceeded this method will
// set the result to the default value (which can be a pointer, but must not be nil), and return
// exchange.WarningHTTPRequestRateLimited.
func (b *Binance) SendRateLimitedHTTPRequest(requestsPerMin uint, method string, path string,
	params url.Values, security RequestSecurityEnum, result interface{}, defaultValue interface{}) error {
	curTimestamp := time.Now().UnixNano() / (1000 * 1000) // convert to milliseconds
	requestDelay := int64((60 * 1000) / requestsPerMin)   // min delay between requests in msecs
	lastRequestTime := b.rateLimits[method+path]
	// If we got IP banned wait 5 mins before trying again, otherwise we might get banned for longer.
	skipRequest := (b.ipBanStartTime != 0) && ((curTimestamp - b.ipBanStartTime) < (5 * 60 * 1000))
	// Make sure requests are spaced out to avoid getting IP banned in the first place.
	if !skipRequest {
		skipRequest = (curTimestamp - lastRequestTime) < requestDelay
	}

	if !skipRequest {
		code, err := b.SendHTTPRequest(method, path, params, security, result)
		if err != nil {
			if BinanceErrCode(code) == TooManyRequestsErrCode {
				b.ipBanStartTime = curTimestamp
				skipRequest = true
			} else {
				return err
			}
		} else {
			b.ipBanStartTime = 0
			b.rateLimits[method+path] = curTimestamp
		}
	}

	if skipRequest {
		// set result to default value
		rv := reflect.ValueOf(result)
		if rv.Kind() != reflect.Ptr || rv.IsNil() {
			return errors.New("result must be a non-nil pointer")
		}
		dv := reflect.ValueOf(defaultValue)
		if !dv.IsValid() {
			return errors.New("default value must not be nil")
		}
		if dv.Kind() == reflect.Ptr {
			reflect.Indirect(rv).Set(dv.Elem())
		} else {
			reflect.Indirect(rv).Set(dv)
		}

		return exchange.WarningHTTPRequestRateLimited()
	}

	return nil
}
