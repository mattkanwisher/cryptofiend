package exchange

import "github.com/mattkanwisher/cryptofiend/currency/pair"

// ILimits provides information about the limits placed by an exchange on numbers representing
// order/trade price and amount.
type ILimits interface {
	// Returns max number of decimal places allowed in the trade price for the given currency pair,
	// -1 should be used to indicate this value isn't defined.
	GetPriceDecimalPlaces(p pair.CurrencyPair) int32
	// Returns max number of decimal places allowed in the trade amount for the given currency pair,
	// -1 should be used to indicate this value isn't defined.
	GetAmountDecimalPlaces(p pair.CurrencyPair) int32
	// Returns the minimum trade amount for the given currency pair.
	GetMinAmount(p pair.CurrencyPair) float64
}

// DefaultExchangeLimits provides reasonable defaults for exchanges that don't bother specifying
// this kind of information in their API docs.
type DefaultExchangeLimits struct{}

// Returns max number of decimal places allowed in the trade price for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (l *DefaultExchangeLimits) GetPriceDecimalPlaces(p pair.CurrencyPair) int32 {
	return 8
}

// Returns max number of decimal places allowed in the trade amount for the given currency pair,
// -1 should be used to indicate this value isn't defined.
func (l *DefaultExchangeLimits) GetAmountDecimalPlaces(p pair.CurrencyPair) int32 {
	return 8
}

// Returns the minimum trade amount for the given currency pair.
func (l *DefaultExchangeLimits) GetMinAmount(p pair.CurrencyPair) float64 {
	return 0.00000001
}
