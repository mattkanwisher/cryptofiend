package bittrex

import (
	"log"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
	"github.com/shopspring/decimal"
)

// Start starts the Bittrex go routine
func (b *Bittrex) Start() {
	go b.Run()
}

// Run implements the Bittrex wrapper
func (b *Bittrex) Run() {
	if b.Verbose {
		log.Printf("%s polling delay: %ds.\n", b.GetName(), b.RESTPollingDelay)
		log.Printf("%s %d currencies enabled: %s.\n", b.GetName(), len(b.EnabledPairs), b.EnabledPairs)
	}

	exchangeProducts, err := b.GetMarkets()
	if err != nil {
		log.Printf("%s Failed to get available symbols.\n", b.GetName())
	} else {
		b.currencyPairs = make(map[pair.CurrencyItem]*exchange.CurrencyPairInfo, len(exchangeProducts))
		b.minTradeSizes = make(map[pair.CurrencyItem]float64, len(exchangeProducts))
		for i := range exchangeProducts {
			market := &exchangeProducts[i]
			// Bittrex doesn't follow common conventions for currency pairs, it inverts the
			// currencies for some bizare reason, e.g. ETH/BTC on other exchanges corresponds
			// to BTC/ETH on Bittrex.
			currencyPair := b.SymbolToCurrencyPair(market.MarketName)
			b.currencyPairs[pair.CurrencyItem(market.MarketName)] = &exchange.CurrencyPairInfo{
				Currency:           currencyPair,
				FirstCurrencyName:  market.MarketCurrencyLong,
				SecondCurrencyName: market.BaseCurrencyLong,
			}
			b.minTradeSizes[currencyPair.Display("/", false)] = market.MinTradeSize
		}

		forceUpgrade := false
		if !common.DataContains(b.EnabledPairs, "-") || !common.DataContains(b.AvailablePairs, "-") {
			forceUpgrade = true
		}
		var currencies []string
		for x := range exchangeProducts {
			if !exchangeProducts[x].IsActive || exchangeProducts[x].MarketName == "" {
				continue
			}
			currencies = append(currencies, exchangeProducts[x].MarketName)
		}

		if forceUpgrade {
			enabledPairs := []string{"USDT-BTC"}
			log.Println("WARNING: Available pairs for Bittrex reset due to config upgrade, please enable the ones you would like again")

			err = b.UpdateEnabledCurrencies(enabledPairs, true)
			if err != nil {
				log.Printf("%s Failed to get config.\n", b.GetName())
			}
		}
		err = b.UpdateAvailableCurrencies(currencies, forceUpgrade)
		if err != nil {
			log.Printf("%s Failed to get config.\n", b.GetName())
		}
	}
}

// GetExchangeAccountInfo Retrieves balances for all enabled currencies for the
// Bittrex exchange
func (b *Bittrex) GetExchangeAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = b.GetName()
	accountBalance, err := b.GetAccountBalances()
	if err != nil {
		return response, err
	}

	for i := 0; i < len(accountBalance); i++ {
		src := &accountBalance[i]
		exchangeCurrency := exchange.AccountCurrencyInfo{
			CurrencyName: src.Currency,
			TotalValue:   src.Balance,
			Available:    src.Available,
		}
		exchangeCurrency.Hold, _ = decimal.NewFromFloat(src.Balance).
			Sub(decimal.NewFromFloat(src.Available)).Float64()
		response.Currencies = append(response.Currencies, exchangeCurrency)
	}
	return response, nil
}

// UpdateTicker updates and returns the ticker for a currency pair
func (b *Bittrex) UpdateTicker(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	var tickerPrice ticker.Price
	tick, err := b.GetMarketSummary(exchange.FormatExchangeCurrency(b.GetName(), p).String())
	if err != nil {
		return tickerPrice, err
	}
	tickerPrice.Pair = p
	tickerPrice.Ask = tick[0].Ask
	tickerPrice.Bid = tick[0].Bid
	tickerPrice.Last = tick[0].Last
	tickerPrice.Volume = tick[0].Volume
	ticker.ProcessTicker(b.GetName(), p, tickerPrice, assetType)
	return ticker.GetTicker(b.Name, p, assetType)
}

// GetTickerPrice returns the ticker for a currency pair
func (b *Bittrex) GetTickerPrice(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	tick, err := ticker.GetTicker(b.GetName(), p, ticker.Spot)
	if err != nil {
		return b.UpdateTicker(p, assetType)
	}
	return tick, nil
}

// GetOrderbookEx returns the orderbook for a currency pair
func (b *Bittrex) GetOrderbookEx(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	ob, err := b.Orderbooks.GetOrderbook(b.GetName(), p, assetType)
	if err == nil {
		return b.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (b *Bittrex) UpdateOrderbook(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	var orderBook orderbook.Base
	symbol := b.CurrencyPairToSymbol(p)
	orderbookNew, err := b.GetOrderbook(symbol)
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Buy {
		orderBook.Bids = append(orderBook.Bids,
			orderbook.Item{
				Amount: orderbookNew.Buy[x].Quantity,
				Price:  orderbookNew.Buy[x].Rate,
			},
		)
	}

	for x := range orderbookNew.Sell {
		orderBook.Asks = append(orderBook.Asks,
			orderbook.Item{
				Amount: orderbookNew.Sell[x].Quantity,
				Price:  orderbookNew.Sell[x].Rate,
			},
		)
	}

	b.Orderbooks.ProcessOrderbook(b.GetName(), p, orderBook, assetType)
	return b.Orderbooks.GetOrderbook(b.Name, p, assetType)
}

// GetEnabledCurrencies returns the enabled currency pairs for the exchange.
func (b *Bittrex) GetEnabledCurrencies() []pair.CurrencyPair {
	// Bittrex doesn't follow common conventions for currency pairs, it inverts the
	// currencies for some bizare reason, so invert them again here so they're
	// consistent with the other exchanges.
	pairs := b.Base.GetEnabledCurrencies()
	for i := range pairs {
		pairs[i] = pairs[i].Invert()
	}
	return pairs
}

// GetAvailableCurrencies returns the available currency pairs for the exchange.
func (b *Bittrex) GetAvailableCurrencies() []pair.CurrencyPair {
	// Bittrex doesn't follow common conventions for currency pairs, it inverts the
	// currencies for some bizare reason, so invert them again here so they're
	// consistent with the other exchanges.
	pairs := b.Base.GetAvailableCurrencies()
	for i := range pairs {
		pairs[i] = pairs[i].Invert()
	}
	return pairs
}
