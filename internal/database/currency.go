package database

import (
	"bytes"
	"encoding/json"
	"reflect"

	"github.com/constb/tt-golang/internal/proto"
	"github.com/shopspring/decimal"
)

var (
	baseCurrency string
	rates        map[string]decimal.Decimal
)

func init() {
	// loadRates()
	loadRatesFromStub()
}

// TODO: actually load conversion rates from api
// func loadRates() {}

func loadRatesFromStub() {
	v := map[string]any{}
	err := json.NewDecoder(bytes.NewReader([]byte(stub))).Decode(&v)
	if err != nil {
		panic(err)
	}

	base, ok := v["base"].(string)
	if !ok {
		panic(reflect.TypeOf(v["base"]))
	}
	ratesData, ok := v["rates"].(map[string]any)
	if !ok {
		panic(reflect.TypeOf(v["rates"]))
	}

	baseCurrency = base
	rates = make(map[string]decimal.Decimal, len(ratesData))

	for cur, rate := range ratesData {
		rateValue, ok := rate.(float64)
		if !ok {
			panic(reflect.TypeOf(rate))
		}
		rates[cur] = decimal.NewFromFloat(rateValue)
	}
}

func IsCurrencyValid(currency string) bool {
	_, ok := rates[currency]
	return ok
}

func ConvertCurrency(value decimal.Decimal, from, to string) (decimal.Decimal, error) {
	var valueInBase decimal.Decimal
	if value.IsNegative() {
		return decimal.Zero, proto.NewBadParameterError("value")
	}
	if from == to {
		return decimal.Zero, proto.NewBadParameterError("currency")
	}

	if from == baseCurrency {
		valueInBase = value
	} else {
		rate, ok := rates[from]
		if !ok {
			return decimal.Zero, proto.NewInvalidCurrencyError(from)
		}

		if value.IsZero() {
			valueInBase = decimal.Zero
		} else {
			valueInBase = value.Div(rate)
		}
	}

	if to == baseCurrency {
		return valueInBase, nil
	} else {
		rate, ok := rates[to]
		if !ok {
			return decimal.Zero, proto.NewInvalidCurrencyError(to)
		}

		if value.IsZero() {
			return decimal.Zero, nil
		} else {
			return valueInBase.Mul(rate), nil
		}
	}
}

var stub = `{
  "base": "EUR",
  "date": "2022-11-20",
  "rates": {
    "AED": 3.799913,
    "AFN": 91.040516,
    "ALL": 117.162584,
    "AMD": 408.830211,
    "ANG": 1.862988,
    "AOA": 528.802751,
    "ARS": 168.470072,
    "AUD": 1.549912,
    "AWG": 1.862191,
    "AZN": 1.756403,
    "BAM": 1.950345,
    "BBD": 2.08717,
    "BDT": 106.527193,
    "BGN": 1.960269,
    "BHD": 0.389326,
    "BIF": 2123.932057,
    "BMD": 1.03455,
    "BND": 1.41915,
    "BOB": 7.143049,
    "BRL": 5.568784,
    "BSD": 1.033723,
    "BTC": 6.2510943e-05,
    "BTN": 84.429292,
    "BWP": 13.390494,
    "BYN": 2.610758,
    "BYR": 20277.188664,
    "BZD": 2.08368,
    "CAD": 1.387281,
    "CDF": 2114.620859,
    "CHF": 0.987777,
    "CLF": 0.035311,
    "CLP": 974.339479,
    "CNY": 7.3659,
    "COP": 5161.196282,
    "CRC": 631.117254,
    "CUC": 1.03455,
    "CUP": 27.415587,
    "CVE": 110.541277,
    "CZK": 24.399839,
    "DJF": 183.860512,
    "DKK": 7.453415,
    "DOP": 56.385041,
    "DZD": 143.858337,
    "EGP": 25.412711,
    "ERN": 15.518257,
    "ETB": 54.800194,
    "EUR": 1,
    "FJD": 2.307824,
    "FKP": 0.869885,
    "GBP": 0.870211,
    "GEL": 2.814406,
    "GGP": 0.869885,
    "GHS": 15.000588,
    "GIP": 0.869885,
    "GMD": 63.626077,
    "GNF": 9078.179937,
    "GTQ": 8.071317,
    "GYD": 216.27176,
    "HKD": 8.092202,
    "HNL": 25.708943,
    "HRK": 7.526667,
    "HTG": 142.655555,
    "HUF": 407.343721,
    "IDR": 16181.817284,
    "ILS": 3.586375,
    "IMP": 0.869885,
    "INR": 84.336707,
    "IQD": 1510.443645,
    "IRR": 43864.938367,
    "ISK": 149.202686,
    "JEP": 0.869885,
    "JMD": 158.931096,
    "JOD": 0.733449,
    "JPY": 145.193963,
    "KES": 126.370682,
    "KGS": 87.360855,
    "KHR": 4283.038765,
    "KMF": 492.960672,
    "KPW": 931.095413,
    "KRW": 1386.328965,
    "KWD": 0.318431,
    "KYD": 0.861407,
    "KZT": 477.53698,
    "LAK": 19087.455773,
    "LBP": 850.922872,
    "LKR": 379.902419,
    "LRD": 159.320487,
    "LSL": 17.97059,
    "LTL": 3.054759,
    "LVL": 0.625789,
    "LYD": 5.064168,
    "MAD": 11.08469,
    "MDL": 19.821741,
    "MGA": 4466.154127,
    "MKD": 61.442201,
    "MMK": 2170.870312,
    "MNT": 3531.935,
    "MOP": 8.328658,
    "MRO": 369.33433,
    "MUR": 45.147845,
    "MVR": 15.947597,
    "MWK": 1059.379798,
    "MXN": 20.112179,
    "MYR": 4.711139,
    "MZN": 66.035672,
    "NAD": 17.969926,
    "NGN": 457.953764,
    "NIO": 37.243989,
    "NOK": 10.545328,
    "NPR": 135.088648,
    "NZD": 1.681513,
    "OMR": 0.397771,
    "PAB": 1.033713,
    "PEN": 3.940344,
    "PGK": 3.641493,
    "PHP": 59.183546,
    "PKR": 230.13581,
    "PLN": 4.708911,
    "PYG": 7412.496478,
    "QAR": 3.766285,
    "RON": 4.951383,
    "RSD": 117.32505,
    "RUB": 62.952522,
    "RWF": 1091.450716,
    "SAR": 3.888389,
    "SBD": 8.514963,
    "SCR": 14.975175,
    "SDG": 589.17846,
    "SEK": 11.003891,
    "SGD": 1.423647,
    "SHP": 1.424989,
    "SLE": 18.715042,
    "SLL": 18647.772106,
    "SOS": 588.14321,
    "SRD": 31.699142,
    "STD": 21413.105401,
    "SVC": 9.044563,
    "SYP": 2599.337015,
    "SZL": 17.970319,
    "THB": 37.046883,
    "TJS": 10.551326,
    "TMT": 3.631272,
    "TND": 3.273836,
    "TOP": 2.454315,
    "TRY": 19.264674,
    "TTD": 7.016602,
    "TWD": 32.201729,
    "TZS": 2412.571578,
    "UAH": 38.177783,
    "UGX": 3860.941532,
    "USD": 1.03455,
    "UYU": 41.131521,
    "UZS": 11607.656055,
    "VEF": 1011298.968166,
    "VND": 25664.610091,
    "VUV": 122.823144,
    "WST": 2.881748,
    "XAF": 654.133131,
    "XAG": 0.049449,
    "XAU": 0.000591,
    "XCD": 2.795925,
    "XDR": 0.788801,
    "XOF": 663.672391,
    "XPF": 119.645986,
    "YER": 258.922152,
    "ZAR": 17.856723,
    "ZMK": 9312.187622,
    "ZMW": 17.227433,
    "ZWL": 333.12482
  },
  "success": true,
  "timestamp": 1668963843
}`
