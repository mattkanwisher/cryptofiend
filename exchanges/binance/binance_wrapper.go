package binance

import (
	"log"
	"strconv"
	"strings"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/config"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
	"github.com/shopspring/decimal"
)

// SetDefaults sets the basic defaults for Binance
func (b *Binance) SetDefaults() {
	b.Name = "Binance"
	b.Enabled = false
	b.Verbose = false
	b.Websocket = false
	b.RESTPollingDelay = 10
	b.RequestCurrencyPairFormat.Delimiter = ""
	b.RequestCurrencyPairFormat.Uppercase = true
	b.ConfigCurrencyPairFormat.Delimiter = ""
	b.ConfigCurrencyPairFormat.Uppercase = true
	b.AssetTypes = []string{ticker.Spot}
	b.Orderbooks = orderbook.Init()
	b.rateLimits = map[string]*rateLimitInfo{}
}

// Setup takes in the supplied exchange configuration details and sets params
func (b *Binance) Setup(exch config.ExchangeConfig) {
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

// Start starts the Binance go routine
func (b *Binance) Start() {
	go b.Run()
}

// Run implements the Binance wrapper
func (b *Binance) Run() {
	if b.Verbose {
		log.Printf("%s polling delay: %ds.\n", b.GetName(), b.RESTPollingDelay)
		log.Printf("%s %d currencies enabled: %s.\n", b.GetName(), len(b.EnabledPairs), b.EnabledPairs)
	}

	exchangeInfo, err := b.GetExchangeInfo()
	if err != nil {
		log.Printf("%s failed to get exchange info\n", b.GetName())
		return
	}

	exchangeProducts := make([]string, len(exchangeInfo.Symbols))
	b.currencyPairs = make(map[pair.CurrencyItem]*exchange.CurrencyPairInfo, len(exchangeInfo.Symbols))
	//b.symbolDetails = make(map[pair.CurrencyItem]*SymbolDetails, len(symbolsDetails))
	for i := range exchangeInfo.Symbols {
		symbolInfo := &exchangeInfo.Symbols[i]
		exchangeProducts[i] = symbolInfo.Symbol
		currencyPair := pair.NewCurrencyPair(symbolInfo.BaseAsset, symbolInfo.QuoteAsset)
		b.currencyPairs[pair.CurrencyItem(symbolInfo.Symbol)] = &exchange.CurrencyPairInfo{Currency: currencyPair}
		//b.symbolDetails[currencyPair.Display("/", false)] = symbolInfo
	}
	err = b.UpdateAvailableCurrencies(exchangeProducts, false)
	if err != nil {
		log.Printf("%s failed to update available currencies\n", b.Name)
	}
}

// UpdateTicker updates and returns the ticker for a currency pair
func (b *Binance) UpdateTicker(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	panic("not implemented")
}

// GetTickerPrice returns the ticker for a currency pair
func (b *Binance) GetTickerPrice(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	panic("not implemented")
}

// GetOrderbookEx returns the orderbook for a currency pair
func (b *Binance) GetOrderbookEx(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	ob, err := b.Orderbooks.GetOrderbook(b.GetName(), p, assetType)
	if err == nil {
		return b.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (b *Binance) UpdateOrderbook(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	panic("not implemented")
}

// GetExchangeAccountInfo retrieves balances for all enabled currencies on the
// Binance exchange
func (b *Binance) GetExchangeAccountInfo() (exchange.AccountInfo, error) {
	result := exchange.AccountInfo{}
	result.ExchangeName = b.Name

	if !b.Enabled {
		return result, nil
	}

	accountInfo, err := b.GetAccountInfo()
	if err != nil {
		return result, err
	}
	result.Currencies = make([]exchange.AccountCurrencyInfo, len(accountInfo.Balances))
	for i, src := range accountInfo.Balances {
		dest := &result.Currencies[i]
		dest.CurrencyName = src.Asset
		dest.Hold = src.Locked
		dest.TotalValue, _ = decimal.NewFromFloat(src.Free).Add(decimal.NewFromFloat(src.Locked)).Float64()
	}
	return result, nil
}

// NewOrder creates a new order on the exchange.
// Returns the ID of the new exchange order, or an empty string if the order was filled
// immediately but no ID was generated.
func (b *Binance) NewOrder(symbol pair.CurrencyPair, amount, price float64, side exchange.OrderSide,
	orderType exchange.OrderType) (string, error) {
	panic("not implemented")
}

// CancelOrder will attempt to cancel the active order matching the given ID.
func (b *Binance) CancelOrder(OrderID string) error {
	panic("not implemented")
}

// GetOrder returns information about a previously placed order (which may be active or inactive).
func (b *Binance) GetOrder(orderID string) (*exchange.Order, error) {
	panic("not implemented")
}

// GetOrders returns information about currently active orders.
func (b *Binance) GetOrders() ([]*exchange.Order, error) {
	orders, err := b.GetOpenOrders()
	if err != nil {
		return nil, err
	}
	ret := make([]*exchange.Order, 0, len(orders))
	for _, order := range orders {
		ret = append(ret, b.convertOrderToExchangeOrder(&order))
	}
	return ret, nil
}

func (b *Binance) convertOrderToExchangeOrder(order *Order) *exchange.Order {
	retOrder := &exchange.Order{}
	retOrder.OrderID = strconv.FormatInt(order.OrderID, 10)

	switch order.Status {
	case OrderStatusCanceled, OrderStatusExpired, OrderStatusRejected, OrderStatusReplaced:
		retOrder.Status = exchange.OrderStatusAborted
	case OrderStatusFilled:
		retOrder.Status = exchange.OrderStatusFilled
	default:
		if order.IsWorking {
			retOrder.Status = exchange.OrderStatusActive
		} else {
			retOrder.Status = exchange.OrderStatusUnknown
		}
	}

	retOrder.Amount = order.OrigQty
	retOrder.FilledAmount = order.ExecutedQty
	retOrder.RemainingAmount, _ = decimal.NewFromFloat(order.OrigQty).Sub(decimal.NewFromFloat(order.ExecutedQty)).Float64()
	retOrder.Rate = order.Price
	retOrder.CreatedAt = order.Time / 1000 // Binance specifies timestamps in milliseconds, convert it to seconds
	retOrder.CurrencyPair, _ = b.SymbolToCurrencyPair(order.Symbol)
	retOrder.Side = exchange.OrderSide(strings.ToLower(string(order.Side)))
	if order.Type == OrderTypeLimit {
		retOrder.Type = exchange.OrderTypeExchangeLimit
	} else {
		log.Printf("Binance.convertOrderToExchangeOrder(): unexpected '%s' order", order.Type)
	}

	return retOrder
}

// GetLimits returns price/amount limits for the exchange.
func (b *Binance) GetLimits() exchange.ILimits {
	panic("not implemented")
}

// Returns currency pairs that can be used by the exchange account associated with this bot.
// Use FormatExchangeCurrency to get the right key.
func (b *Binance) GetCurrencyPairs() map[pair.CurrencyItem]*exchange.CurrencyPairInfo {
	return b.currencyPairs
}
