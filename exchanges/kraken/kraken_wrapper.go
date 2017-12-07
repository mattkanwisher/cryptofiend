package kraken

import (
	"log"
	"strings"

	"github.com/mattkanwisher/cryptofiend/currency/pair"
	"github.com/mattkanwisher/cryptofiend/exchanges"
	"github.com/mattkanwisher/cryptofiend/exchanges/orderbook"
	"github.com/mattkanwisher/cryptofiend/exchanges/ticker"
)

// Start starts the Kraken go routine
func (k *Kraken) Start() {
	go k.Run()
}

// Run implements the Kraken wrapper
func (k *Kraken) Run() {
	if k.Verbose {
		log.Printf("%s polling delay: %ds.\n", k.GetName(), k.RESTPollingDelay)
		log.Printf("%s %d currencies enabled: %s.\n", k.GetName(), len(k.EnabledPairs), k.EnabledPairs)
	}

	assets, err := k.GetAssets()
	if err != nil {
		log.Printf("failed to fetch assets from %s\n", k.Name)
		return
	}
	// Map Kraken asset name to currency code, e.g. XLTC->LTC
	// TODO: should probably map XXBT->BTC instead of XXBT->XBT for consistency with other exchanges
	assetNameToCurrency := make(map[string]string, len(assets))
	for assetName, assetInfo := range assets {
		assetNameToCurrency[assetName] = assetInfo.AltName
	}

	assetPairs, err := k.GetAssetPairs()
	if err != nil {
		log.Printf("failed to fetch asset pairs from %s\n", k.GetName())
		return
	}

	k.CurrencyPairCodeToSymbol = make(map[pair.CurrencyItem]string, len(assetPairs))
	k.CurrencyPairs = make(map[pair.CurrencyItem]*exchange.CurrencyPairInfo, len(assetPairs))
	var exchangeProducts []string
	for assetPairName, assetPairInfo := range assetPairs {
		exchangeProducts = append(exchangeProducts, assetPairInfo.Altname)
		// Skip the dark pool asset pairs for now
		if strings.HasSuffix(assetPairName, ".d") {
			continue
		}
		c1, exists := assetNameToCurrency[assetPairInfo.Base]
		if !exists {
			log.Printf("failed to map Kraken asset name '%s' to a currency code", assetPairInfo.Base)
			continue
		}
		c2, exists := assetNameToCurrency[assetPairInfo.Quote]
		if !exists {
			log.Printf("failed to map Kraken asset name '%s' to a currency code", assetPairInfo.Quote)
			continue
		}
		currencyPair := pair.NewCurrencyPair(c1, c2).
			FormatPair(k.RequestCurrencyPairFormat.Delimiter, k.RequestCurrencyPairFormat.Uppercase)
		k.CurrencyPairCodeToSymbol[currencyPair.Display("/", true)] = assetPairName
		k.CurrencyPairs[pair.CurrencyItem(assetPairName)] = &exchange.CurrencyPairInfo{Currency: currencyPair}
	}
	err = k.UpdateAvailableCurrencies(exchangeProducts, false)
	if err != nil {
		log.Printf("%s Failed to get config.\n", k.GetName())
	}
}

// UpdateTicker updates and returns the ticker for a currency pair
func (k *Kraken) UpdateTicker(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	var tickerPrice ticker.Price
	pairs := k.GetEnabledCurrencies()
	pairsCollated, err := exchange.GetAndFormatExchangeCurrencies(k.Name, pairs)
	if err != nil {
		return tickerPrice, err
	}
	err = k.GetTicker(pairsCollated.String())
	if err != nil {
		return tickerPrice, err
	}

	for _, x := range pairs {
		var tp ticker.Price
		tick, ok := k.Ticker[x.Pair().String()]
		if !ok {
			continue
		}

		tp.Pair = x
		tp.Last = tick.Last
		tp.Ask = tick.Ask
		tp.Bid = tick.Bid
		tp.High = tick.High
		tp.Low = tick.Low
		tp.Volume = tick.Volume
		ticker.ProcessTicker(k.GetName(), x, tp, assetType)
	}
	return ticker.GetTicker(k.GetName(), p, assetType)
}

// GetTickerPrice returns the ticker for a currency pair
func (k *Kraken) GetTickerPrice(p pair.CurrencyPair, assetType string) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(k.GetName(), p, assetType)
	if err != nil {
		return k.UpdateTicker(p, assetType)
	}
	return tickerNew, nil
}

// GetOrderbookEx returns orderbook base on the currency pair
func (k *Kraken) GetOrderbookEx(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	ob, err := k.Orderbooks.GetOrderbook(k.GetName(), p, assetType)
	if err == nil {
		return k.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (k *Kraken) UpdateOrderbook(p pair.CurrencyPair, assetType string) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := k.GetDepth(exchange.FormatExchangeCurrency(k.GetName(), p).String())
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Bids {
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: orderbookNew.Bids[x].Amount, Price: orderbookNew.Bids[x].Price})
	}

	for x := range orderbookNew.Asks {
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: orderbookNew.Asks[x].Amount, Price: orderbookNew.Asks[x].Price})
	}

	k.Orderbooks.ProcessOrderbook(k.GetName(), p, orderBook, assetType)
	return k.Orderbooks.GetOrderbook(k.Name, p, assetType)
}

// GetExchangeAccountInfo retrieves balances for all enabled currencies for the
// Kraken exchange - to-do
func (k *Kraken) GetExchangeAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = k.GetName()
	return response, nil
}
