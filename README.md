# Cryptocurrency trading bot written in Golang

[![Build Status](https://travis-ci.org/mattkanwisher/cryptofiend.svg?branch=master)](https://travis-ci.org/mattkanwisher/cryptofiend)
[![Software License](https://img.shields.io/badge/License-MIT-orange.svg?style=flat-square)](https://github.com/mattkanwisher/cryptofiend/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/mattkanwisher/cryptofiend?status.svg)](https://godoc.org/github.com/mattkanwisher/cryptofiend)
[![Coverage Status](http://codecov.io/github/mattkanwisher/cryptofiend/coverage.svg?branch=master)](http://codecov.io/github/mattkanwisher/cryptofiend?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/mattkanwisher/cryptofiend)](https://goreportcard.com/report/github.com/mattkanwisher/cryptofiend)

A cryptocurrency trading bot supporting multiple exchanges written in Golang.

**Please note that this bot is under development and is not ready for production!**

## Community

Join our slack to discuss all things related to GoCryptoTrader! [GoCryptoTrader Slack](https://gocryptotrader.herokuapp.com/)

## Exchange Support Table

| Exchange | REST API | Streaming API | FIX API |
|----------|------|-----------|-----|
| Alphapoint | Yes  | Yes        | NA  |
| ANXPRO | Yes  | No        | NA  |
| Bitfinex | Yes  | Yes        | NA  |
| Bitstamp | Yes  | Yes       | NA  |
| Bittrex | Yes | No | NA |
| BTCC | Yes  | Yes     | No  |
| BTCMarkets | Yes | NA       | NA  |
| COINUT | Yes | No | NA |
| GDAX(Coinbase) | Yes | Yes | No|
| Gemini | Yes | NA | NA |
| Huobi | Yes | Yes |No |
| ItBit | Yes | NA | NA |
| Kraken | Yes | NA | NA |
| LakeBTC | Yes | No | NA |
| Liqui | Yes | No | NA |
| LocalBitcoins | Yes | NA | NA |
| OKCoin (both) | Yes | Yes | No |
| Poloniex | Yes | Yes | NA |
| WEX     | Yes  | NA        | NA  |

We are aiming to support the top 20 highest volume exchanges based off the [CoinMarketCap exchange data](https://coinmarketcap.com/exchanges/volume/24-hour/).

** NA means not applicable as the Exchange does not support the feature.

## Current Features

+ Support for all Exchange fiat and digital currencies, with the ability to individually toggle them on/off.
+ AES encrypted config file.
+ REST API support for all exchanges.
+ Websocket support for applicable exchanges.
+ Ability to turn off/on certain exchanges.
+ Ability to adjust manual polling timer for exchanges.
+ SMS notification support via SMS Gateway.
+ Packages for handling currency pairs, ticker/orderbook fetching and currency conversion.
+ Portfolio management tool; fetches balances from supported exchanges and allows for custom address tracking.
+ Basic event trigger system.
+ WebGUI.

## Planned Features

Planned features can be found on our [community Trello page](https://trello.com/b/ZAhMhpOy/gocryptotrader).

## Contribution

Please feel free to submit any pull requests or suggest any desired features to be added.

When submitting a PR, please abide by our coding guidelines:

+ Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
+ Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary) guidelines.
+ Code must adhere to our [coding style](https://github.com/mattkanwisher/cryptofiend/blob/master/doc/coding_style.md).
+ Pull requests need to be based on and opened against the `master` branch.

## Compiling instructions

Download and install Go from [Go Downloads](https://golang.org/dl/)

```
go get github.com/mattkanwisher/cryptofiend
cd $GOPATH/src/github.com/mattkanwisher/cryptofiend
go install
cp $GOPATH/src/github.com/mattkanwisher/cryptofiend/config_example.dat $GOPATH/bin/config.dat
```

Make any neccessary changes to the config file.
Run the application!

## Donations

If this framework helped you in any way, or you would like to support the developers working on it, please donate Bitcoin to: 1F5zVDgNjorJ51oGebSvNCrSAHpwGkUdDB

## Binaries

Binaries will be published once the codebase reaches a stable condition.
