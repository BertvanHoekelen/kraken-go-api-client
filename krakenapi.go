package krakenapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// APIURL is the official Kraken API Endpoint
	APIURL = "https://api.kraken.com"
	// APIVersion is the official Kraken API Version Number
	APIVersion = "0"
	// APIUserAgent identifies this library with the Kraken API
	APIUserAgent = "Kraken GO API Agent (https://github.com/beldur/kraken-go-api-client)"
)

// List of valid public methods
var publicMethods = []string{
	"Time",
	"Assets",
	"AssetPairs",
	"Ticker",
	"Depth",
	"Trades",
	"Spread",
}

// List of valid private methods
var privateMethods = []string{
	"Balance",
	"TradeBalance",
	"OpenOrders",
	"ClosedOrders",
	"QueryOrders",
	"TradesHistory",
	"QueryTrades",
	"OpenPositions",
	"Ledgers",
	"QueryLedgers",
	"TradeVolume",
	"AddOrder",
	"CancelOrder",
}

// KrakenApi represents a Kraken API Client connection
type KrakenApi struct {
	key    string
	secret string
	client *http.Client
}

// New creates a new Kraken API client
func New(key, secret string) *KrakenApi {
	client := &http.Client{}

	return &KrakenApi{key, secret, client}
}

// Time returns the server's time
func (api *KrakenApi) Time() (*TimeResponse, error) {
	resp, err := api.queryPublic("Time", nil, &TimeResponse{})
	if err != nil {
		return nil, err
	}

	return resp.(*TimeResponse), nil
}

// Assets returns the servers available assets
func (api *KrakenApi) Assets() (*AssetsResponse, error) {
	resp, err := api.queryPublic("Assets", nil, &AssetsResponse{})
	if err != nil {
		return nil, err
	}

	return resp.(*AssetsResponse), nil
}

// AssetPairs returns the servers available asset pairs
func (api *KrakenApi) AssetPairs() (*AssetPairsResponse, error) {
	resp, err := api.queryPublic("AssetPairs", nil, &AssetPairsResponse{})
	if err != nil {
		return nil, err
	}

	return resp.(*AssetPairsResponse), nil
}

// Ticker returns the ticker for given comma separated pairs
func (api *KrakenApi) Ticker(pairs ...string) (*TickerResponse, error) {
	resp, err := api.queryPublic("Ticker", url.Values{
		"pair": {strings.Join(pairs, ",")},
	}, &TickerResponse{})
	if err != nil {
		return nil, err
	}

	return resp.(*TickerResponse), nil
}

// Trades returns the recent trades for given pair
func (api *KrakenApi) Trades(pair string, since int64) (*TradesResponse, error) {
	values := url.Values{"pair": {pair}}
	if since > 0 {
		values.Set("since", strconv.FormatInt(since, 10))
	}
	resp, err := api.queryPublic("Trades", values, nil)
	if err != nil {
		return nil, err
	}

	v := resp.(map[string]interface{})

	last, err := strconv.ParseInt(v["last"].(string), 10, 64)
	if err != nil {
		return nil, err
	}

	result := &TradesResponse{
		Last:   last,
		Trades: make([]TradeInfo, 0),
	}

	trades := v[pair].([]interface{})
	for _, v := range trades {
		trade := v.([]interface{})
		priceString := trade[0].(string)
		price, _ := strconv.ParseFloat(priceString, 64)
		volume, _ := strconv.ParseFloat(trade[1].(string), 64)
		priceParts := strings.Split(priceString, ".")
		priceInt, _ := strconv.ParseInt(priceParts[0]+priceParts[1], 10, 64)

		tradeInfo := TradeInfo{
			Price:         trade[0].(string),
			PriceFloat:    price,
			PriceInt:      priceInt,
			Volume:        volume,
			Time:          trade[2].(float64),
			Buy:           trade[3].(string) == BUY,
			Sell:          trade[3].(string) == SELL,
			Market:        trade[4].(string) == MARKET,
			Limit:         trade[4].(string) == LIMIT,
			Miscellaneous: trade[5].(string),
		}

		result.Trades = append(result.Trades, tradeInfo)
	}

	return result, nil
}

// Balance returns all account asset balances
func (api *KrakenApi) Balance() (*BalanceResponse, error) {
	resp, err := api.queryPrivate("Balance", url.Values{}, &BalanceResponse{})
	if err != nil {
		return nil, err
	}

	return resp.(*BalanceResponse), nil
}

// Query sends a query to Kraken api for given method and parameters
func (api *KrakenApi) Query(method string, data map[string]string) (interface{}, error) {
	values := url.Values{}
	for key, value := range data {
		values.Set(key, value)
	}

	// Check if method is public or private
	if isStringInSlice(method, publicMethods) {
		return api.queryPublic(method, values, nil)
	} else if isStringInSlice(method, privateMethods) {
		return api.queryPrivate(method, values, nil)
	}

	return nil, fmt.Errorf("Method '%s' is not valid", method)
}

// Execute a public method query
func (api *KrakenApi) queryPublic(method string, values url.Values, typ interface{}) (interface{}, error) {
	url := fmt.Sprintf("%s/%s/public/%s", APIURL, APIVersion, method)
	resp, err := api.doRequest(url, values, nil, typ)

	return resp, err
}

// queryPrivate executes a private method query
func (api *KrakenApi) queryPrivate(method string, values url.Values, typ interface{}) (interface{}, error) {
	urlPath := fmt.Sprintf("/%s/private/%s", APIVersion, method)
	reqURL := fmt.Sprintf("%s%s", APIURL, urlPath)
	secret, _ := base64.StdEncoding.DecodeString(api.secret)
	values.Set("nonce", fmt.Sprintf("%d", time.Now().UnixNano()))

	// Create signature
	signature := createSignature(urlPath, values, secret)

	// Add Key and signature to request headers
	headers := map[string]string{
		"API-Key":  api.key,
		"API-Sign": signature,
	}

	resp, err := api.doRequest(reqURL, values, headers, typ)

	return resp, err
}

// doRequest executes a HTTP Request to the Kraken API and returns the result
func (api *KrakenApi) doRequest(reqURL string, values url.Values, headers map[string]string, typ interface{}) (interface{}, error) {

	// Create request
	req, err := http.NewRequest("POST", reqURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("Could not execute request! (%s)", err.Error())
	}

	req.Header.Add("User-Agent", APIUserAgent)
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	// Execute request
	resp, err := api.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Could not execute request! (%s)", err.Error())
	}
	defer resp.Body.Close()

	// Read request
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Could not execute request! (%s)", err.Error())
	}

	// Parse request
	var jsonData KrakenResponse

	// Set the KrakenResoinse.Result to typ so `json.Unmarshal` will
	// unmarshal it into given typ instead of `interface{}`.
	if typ != nil {
		jsonData.Result = typ
	}

	err = json.Unmarshal(body, &jsonData)
	if err != nil {
		return nil, fmt.Errorf("Could not execute request! (%s)", err.Error())
	}

	// Check for Kraken API error
	if len(jsonData.Error) > 0 {
		return nil, fmt.Errorf("Could not execute request! (%s)", jsonData.Error)
	}

	return jsonData.Result, nil
}

// isStringInSlice is a helper function to test if given term is in a list of strings
func isStringInSlice(term string, list []string) bool {
	for _, found := range list {
		if term == found {
			return true
		}
	}
	return false
}

// getSha256 creates a sha256 hash for given []byte
func getSha256(input []byte) []byte {
	sha := sha256.New()
	sha.Write(input)
	return sha.Sum(nil)
}

// getHMacSha512 creates a hmac hash with sha512
func getHMacSha512(message, secret []byte) []byte {
	mac := hmac.New(sha512.New, secret)
	mac.Write(message)
	return mac.Sum(nil)
}

func createSignature(urlPath string, values url.Values, secret []byte) string {
	// See https://www.kraken.com/help/api#general-usage for more information
	shaSum := getSha256([]byte(values.Get("nonce") + values.Encode()))
	macSum := getHMacSha512(append([]byte(urlPath), shaSum...), secret)
	return base64.StdEncoding.EncodeToString(macSum)
}
