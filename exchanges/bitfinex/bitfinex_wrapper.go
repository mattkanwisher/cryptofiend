package bitfinex

import (
	"log"
	"net/url"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
	"github.com/shopspring/decimal"
)

// Start starts the Bitfinex go routine
func (b *Bitfinex) Start() {
	go b.Run()
}

// Run implements the Bitfinex wrapper
func (b *Bitfinex) Run() {
	if b.Verbose {
		log.Printf("%s Websocket: %s.", b.GetName(), common.IsEnabled(b.Websocket))
		log.Printf("%s polling delay: %ds.\n", b.GetName(), b.RESTPollingDelay)
		log.Printf("%s %d currencies enabled: %s.\n", b.GetName(), len(b.EnabledPairs), b.EnabledPairs)
	}

	if b.Websocket {
		go b.WebsocketClient()
	}

	symbolsDetails, err := b.GetSymbolsDetails()
	if err != nil {
		log.Printf("%s Failed to get available symbols.\n", b.GetName())
		return
	}
	exchangeProducts := make([]string, len(symbolsDetails))
	b.currencyPairs = make(map[pair.CurrencyItem]*exchange.CurrencyPairInfo, len(symbolsDetails))
	b.symbolDetails = make(map[pair.CurrencyItem]*SymbolDetails, len(symbolsDetails))
	for i := range symbolsDetails {
		symbolInfo := &symbolsDetails[i]
		exchangeProducts[i] = symbolInfo.Pair
		if currencyPair, err := b.SymbolToCurrencyPair(symbolInfo.Pair); err == nil {
			b.currencyPairs[pair.CurrencyItem(symbolInfo.Pair)] = &exchange.CurrencyPairInfo{Currency: currencyPair}
			b.symbolDetails[currencyPair.Display("/", false)] = symbolInfo
		} else {
			log.Print("% failed to convert %s to currency pair", b.GetName(), symbolInfo.Pair)
		}
	}
	err = b.UpdateAvailableCurrencies(exchangeProducts, false)
	if err != nil {
		log.Printf("%s Failed to get config.\n", b.GetName())
	}
}

// UpdateTicker updates and returns the ticker for a currency pair
func (b *Bitfinex) UpdateTicker(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	var tickerPrice ticker.Price
	tickerNew, err := b.GetTicker(p.Pair().String(), nil)
	if err != nil {
		return tickerPrice, err
	}

	tickerPrice.Pair = p
	tickerPrice.Ask = tickerNew.Ask
	tickerPrice.Bid = tickerNew.Bid
	tickerPrice.Low = tickerNew.Low
	tickerPrice.Last = tickerNew.Last
	tickerPrice.Volume = tickerNew.Volume
	tickerPrice.High = tickerNew.High
	ticker.ProcessTicker(b.GetName(), p, tickerPrice, assetType)
	return ticker.GetTicker(b.Name, p, assetType)
}

// GetTickerPrice returns the ticker for a currency pair
func (b *Bitfinex) GetTickerPrice(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	tick, err := ticker.GetTicker(b.GetName(), p, ticker.Spot)
	if err != nil {
		return b.UpdateTicker(p, assetType)
	}
	return tick, nil
}

// GetOrderbookEx returns the orderbook for a currency pair
func (b *Bitfinex) GetOrderbookEx(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	ob, err := b.Orderbooks.GetOrderbook(b.GetName(), p, assetType)
	if err == nil {
		return b.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (b *Bitfinex) UpdateOrderbook(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	var orderBook orderbook.Base
	vals := url.Values{}
	vals.Set("limit_bids", "100")
	vals.Set("limit_asks", "100")
	symbol := b.CurrencyPairToSymbol(p)
	orderbookNew, err := b.GetOrderbook(symbol, vals)
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Asks {
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Price: orderbookNew.Asks[x].Price, Amount: orderbookNew.Asks[x].Amount})
	}

	for x := range orderbookNew.Bids {
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Price: orderbookNew.Bids[x].Price, Amount: orderbookNew.Bids[x].Amount})
	}

	b.Orderbooks.ProcessOrderbook(b.GetName(), p, orderBook, assetType)
	return b.Orderbooks.GetOrderbook(b.Name, p, assetType)
}

// GetExchangeAccountInfo retrieves balances for all enabled currencies on the
// Bitfinex exchange
func (b *Bitfinex) GetExchangeAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = b.GetName()
	accountBalance, err := b.GetAccountBalance()
	if err != nil {
		return response, err
	}
	if !b.Enabled {
		return response, nil
	}

	type walletBalance struct {
		OnHold    decimal.Decimal
		Available decimal.Decimal
	}

	// TODO: Figure out if it makes sense to add up all the wallet balances together or if we
	// should only grab the amounts from the "exchange" wallet.
	accounts := make(map[string]walletBalance)

	for i := range accountBalance {
		src := &accountBalance[i]
		balance := walletBalance{
			OnHold:    decimal.NewFromFloat(src.Amount).Sub(decimal.NewFromFloat(src.Available)),
			Available: decimal.NewFromFloat(src.Available),
		}
		result, ok := accounts[accountBalance[i].Currency]
		if !ok {
			accounts[accountBalance[i].Currency] = balance
		} else {
			result.Available = result.Available.Add(balance.Available)
			result.OnHold = result.OnHold.Add(balance.OnHold)
			accounts[src.Currency] = result
		}
	}

	for currency, src := range accounts {
		exchangeCurrency := exchange.AccountCurrencyInfo{
			CurrencyName: common.StringToUpper(currency),
		}
		exchangeCurrency.Hold, _ = src.OnHold.Float64()
		exchangeCurrency.Available, _ = src.Available.Float64()
		exchangeCurrency.TotalValue, _ = src.Available.Add(src.OnHold).Float64()
		response.Currencies = append(response.Currencies, exchangeCurrency)
	}

	return response, nil
}
