package binance

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	exchange "github.com/mattkanwisher/cryptofiend/exchanges"
)

const (
	binanceBaseURL      = "https://www.binance.com/"
	binanceExchangeInfo = "api/v1/exchangeInfo"
)

type rateLimitInfo struct {
	StartTime    int64
	RequestCount uint
}

type Binance struct {
	exchange.Base
	rateLimits map[string]*rateLimitInfo
	// Maps symbol (exchange specific market identifier) to currency pair info
	currencyPairs map[pair.CurrencyItem]*exchange.CurrencyPairInfo
}

// CurrencyPairToSymbol converts a currency pair to a symbol (exchange specific market identifier).
func (b *Binance) CurrencyPairToSymbol(p pair.CurrencyPair) string {
	return p.
		Display(b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase).
		String()
}

// Fetches current exchange trading rules and symbol information.
func (b *Binance) GetExchangeInfo() (*ExchangeInfo, error) {
	response := ExchangeInfo{}
	err := common.SendHTTPGetRequest(binanceBaseURL+binanceExchangeInfo, true, b.Verbose, &response)
	return &response, err
}

// SendAuthenticatedHTTPRequest sends a POST request to an authenticated endpoint, the response is
// decoded into the result object.
// Returns the Binance error code and error message (if any).
func (b *Binance) SendAuthenticatedHTTPRequest(method, path string, params url.Values,
	result interface{}) (int, error) {
	if !b.AuthenticatedAPISupport {
		return 0, fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, b.Name)
	}

	if b.Verbose {
		log.Printf("Request params: %v\n", params)
	}

	recvWindow := 5000
	timestamp := time.Now().UnixNano() / (1000 * 1000) // must be in milliseconds
	payload := params.Encode() + fmt.Sprintf("&timestamp=%v&recvWindow=%d", timestamp, recvWindow)
	hmac := common.GetHMAC(common.HashSHA256, []byte(payload), []byte(b.APISecret))
	headers := make(http.Header)
	headers["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	headers["Accept"] = []string{"application/json"}
	headers["X-MBX-APIKEY"] = []string{b.APIKey}
	payload = fmt.Sprintf("%s&signature=%s", payload, hex.EncodeToString(hmac))

	resp, statusCode, err := common.SendHTTPRequest2(method, binanceBaseURL+path, headers, strings.NewReader(payload))
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
	}

	return 0, nil
}

// SendRateLimitedHTTPRequest sends an HTTP request if the given number of requests per minute
// hasn't been exceeded for the specified method & path and unmarshals the response into the
// result parameter. If the number of requests per minute has been exceeded this method will
// set the result to the default value (which can be a pointer, but must not be nil).
func (b *Binance) SendRateLimitedHTTPRequest(requestsPerMin uint, method string, path string,
	params url.Values, result interface{}, defaultValue interface{}) error {
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

	_, err := b.SendAuthenticatedHTTPRequest(method, path, params, result)
	return err
}
