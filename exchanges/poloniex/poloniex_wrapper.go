package poloniex

import (
	"log"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
)

// Start starts the Poloniex go routine
func (p *Poloniex) Start() {
	go p.Run()
}

// Run implements the Poloniex wrapper
func (p *Poloniex) Run() {
	if p.Verbose {
		log.Printf("%s Websocket: %s (url: %s).\n", p.GetName(), common.IsEnabled(p.Websocket), POLONIEX_WEBSOCKET_ADDRESS)
		log.Printf("%s polling delay: %ds.\n", p.GetName(), p.RESTPollingDelay)
		log.Printf("%s %d currencies enabled: %s.\n", p.GetName(), len(p.EnabledPairs), p.EnabledPairs)
	}

	if p.Websocket {
		go p.WebsocketClient()
	}

	ticker, err := p.GetTicker()
	if (err != nil) && p.Verbose {
		log.Printf("failed to ticker for %s", p.GetName())
	}
	p.currencyPairs = make(map[pair.CurrencyItem]*exchange.CurrencyPairInfo, len(ticker))
	for symbol := range ticker {
		currencyPair := p.SymbolToCurrencyPair(symbol)
		p.currencyPairs[pair.CurrencyItem(symbol)] = &exchange.CurrencyPairInfo{
			Currency: currencyPair,
		}
	}
}

// UpdateTicker updates and returns the ticker for a currency pair
func (p *Poloniex) UpdateTicker(currencyPair pair.CurrencyPair, assetType string) (ticker.Price, error) {
	var tickerPrice ticker.Price
	tick, err := p.GetTicker()
	if err != nil {
		return tickerPrice, err
	}

	for _, x := range p.GetEnabledCurrencies() {
		var tp ticker.Price
		curr := exchange.FormatExchangeCurrency(p.GetName(), x).String()
		tp.Pair = x
		tp.Ask = tick[curr].LowestAsk
		tp.Bid = tick[curr].HighestBid
		tp.High = tick[curr].High24Hr
		tp.Last = tick[curr].Last
		tp.Low = tick[curr].Low24Hr
		tp.Volume = tick[curr].BaseVolume
		ticker.ProcessTicker(p.GetName(), x, tp, assetType)
	}
	return ticker.GetTicker(p.Name, currencyPair, assetType)
}

// GetTickerPrice returns the ticker for a currency pair
func (p *Poloniex) GetTickerPrice(currencyPair pair.CurrencyPair, assetType string) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(p.GetName(), currencyPair, assetType)
	if err != nil {
		return p.UpdateTicker(currencyPair, assetType)
	}
	return tickerNew, nil
}

// GetOrderbookEx returns orderbook base on the currency pair
func (p *Poloniex) GetOrderbookEx(currencyPair pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	ob, err := p.Orderbooks.GetOrderbook(p.GetName(), currencyPair, assetType)
	if err == nil {
		return p.UpdateOrderbook(currencyPair, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (p *Poloniex) UpdateOrderbook(currencyPair pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	var orderBook orderbook.Base
	symbol := p.CurrencyPairToSymbol(currencyPair)
	orderbookNew, err := p.GetOrderbook(symbol, 1000)

	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Bids {
		data := orderbookNew.Bids[x]
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: data.Amount, Price: data.Price})
	}

	for x := range orderbookNew.Asks {
		data := orderbookNew.Asks[x]
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: data.Amount, Price: data.Price})
	}

	p.Orderbooks.ProcessOrderbook(p.GetName(), currencyPair, orderBook, assetType)
	return p.Orderbooks.GetOrderbook(p.Name, currencyPair, assetType)
}

// GetExchangeAccountInfo retrieves balances for all enabled currencies for the
// Poloniex exchange
func (p *Poloniex) GetExchangeAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = p.GetName()
	accountBalance, err := p.GetBalances()
	if err != nil {
		return response, err
	}

	for x, y := range accountBalance.Currency {
		var exchangeCurrency exchange.AccountCurrencyInfo
		exchangeCurrency.CurrencyName = x
		exchangeCurrency.TotalValue = y
		response.Currencies = append(response.Currencies, exchangeCurrency)
	}
	return response, nil
}

// GetEnabledCurrencies returns the enabled currency pairs for the exchange.
func (p *Poloniex) GetEnabledCurrencies() []pair.CurrencyPair {
	// Poloniex doesn't follow common conventions for currency pairs, it inverts the
	// currencies, so invert them again here so they're consistent with the other exchanges.
	pairs := p.Base.GetEnabledCurrencies()
	for i := range pairs {
		pairs[i] = pairs[i].Invert()
	}
	return pairs
}

// GetAvailableCurrencies returns the available currency pairs for the exchange.
func (p *Poloniex) GetAvailableCurrencies() []pair.CurrencyPair {
	// Poloniex doesn't follow common conventions for currency pairs, it inverts the
	// currencies, so invert them again here so they're consistent with the other exchanges.
	pairs := p.Base.GetAvailableCurrencies()
	for i := range pairs {
		pairs[i] = pairs[i].Invert()
	}
	return pairs
}
