package live

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultOKXBaseURL = "https://www.okx.com"

type OKXClient struct {
	baseURL    string
	key        string
	secret     string
	passphrase string
	http       *http.Client
}

func NewOKXFromEnv(baseURL, keyEnv, secretEnv, passphraseEnv string) (*OKXClient, error) {
	if baseURL == "" {
		baseURL = defaultOKXBaseURL
	}
	if keyEnv == "" {
		keyEnv = "OKX_API_KEY"
	}
	if secretEnv == "" {
		secretEnv = "OKX_API_SECRET"
	}
	if passphraseEnv == "" {
		passphraseEnv = "OKX_API_PASSPHRASE"
	}
	key, err := okxCredentialFromEnv(keyEnv)
	if err != nil {
		return nil, err
	}
	secret, err := okxCredentialFromEnv(secretEnv)
	if err != nil {
		return nil, err
	}
	passphrase, err := okxCredentialFromEnv(passphraseEnv)
	if err != nil {
		return nil, err
	}
	return &OKXClient{baseURL: strings.TrimRight(baseURL, "/"), key: key, secret: secret, passphrase: passphrase, http: &http.Client{Timeout: 30 * time.Second}}, nil
}

func okxCredentialFromEnv(env string) (string, error) {
	value := os.Getenv(env)
	if env == "" || value == "" {
		return "", fmt.Errorf("required OKX credential env is not set: %s", env)
	}
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("OKX credential env contains invalid whitespace/control characters: %s", env)
	}
	return value, nil
}

func (c *OKXClient) InstrumentFilters(ctx context.Context) ([]InstrumentFilter, error) {
	const requestPath = "/api/v5/public/instruments?instType=SPOT"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+requestPath, nil)
	if err != nil {
		return nil, fmt.Errorf("okx instruments request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("okx instruments request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("okx instruments read failed: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("okx instruments http %d: %s", resp.StatusCode, redact(data, c.key, c.secret, c.passphrase))
	}
	filters, err := parseInstrumentFilters(data)
	if err != nil {
		return nil, err
	}
	return filters, nil
}

func (c *OKXClient) PlaceSpotLimitOrder(ctx context.Context, order LimitOrderRequest) (OrderResult, error) {
	const requestPath = "/api/v5/trade/order"
	const method = http.MethodPost
	ordType := "limit"
	if order.PostOnly {
		ordType = "post_only"
	}
	body := map[string]string{
		"instId":  order.InstID,
		"tdMode":  "cash",
		"side":    strings.ToLower(order.Side),
		"ordType": ordType,
		"px":      formatNumber(order.Price),
		"sz":      formatNumber(order.Quantity),
		"clOrdId": order.ClientOrderID,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return OrderResult{}, err
	}
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, bytes.NewReader(b))
	if err != nil {
		return OrderResult{}, fmt.Errorf("okx order request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OK-ACCESS-KEY", c.key)
	req.Header.Set("OK-ACCESS-SIGN", okxSign(timestamp, method, requestPath, string(b), c.secret))
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)

	resp, err := c.http.Do(req)
	if err != nil {
		return OrderResult{}, fmt.Errorf("okx order request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return OrderResult{}, fmt.Errorf("okx order read failed: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return OrderResult{}, fmt.Errorf("okx order http %d: %s", resp.StatusCode, redact(data, c.key, c.secret, c.passphrase))
	}
	result, err := parseOrderResult(data, order.InstID)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (c *OKXClient) CancelOrder(ctx context.Context, cancel CancelOrderRequest) (CancelOrderResult, error) {
	const requestPath = "/api/v5/trade/cancel-order"
	const method = http.MethodPost
	if cancel.InstID == "" {
		return CancelOrderResult{}, fmt.Errorf("instID required")
	}
	body := map[string]string{"instId": cancel.InstID}
	if cancel.OrderID != "" {
		body["ordId"] = cancel.OrderID
	} else if cancel.ClientOrderID != "" {
		body["clOrdId"] = cancel.ClientOrderID
	} else {
		return CancelOrderResult{}, fmt.Errorf("orderID or clientOrderID required")
	}
	b, err := json.Marshal(body)
	if err != nil {
		return CancelOrderResult{}, err
	}
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, bytes.NewReader(b))
	if err != nil {
		return CancelOrderResult{}, fmt.Errorf("okx cancel request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OK-ACCESS-KEY", c.key)
	req.Header.Set("OK-ACCESS-SIGN", okxSign(timestamp, method, requestPath, string(b), c.secret))
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)
	resp, err := c.http.Do(req)
	if err != nil {
		return CancelOrderResult{}, fmt.Errorf("okx cancel request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return CancelOrderResult{}, fmt.Errorf("okx cancel read failed: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return CancelOrderResult{}, fmt.Errorf("okx cancel http %d: %s", resp.StatusCode, redact(data, c.key, c.secret, c.passphrase))
	}
	result, err := parseCancelOrderResult(data, cancel.InstID)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (c *OKXClient) AccountBalance(ctx context.Context) ([]Balance, error) {
	const requestPath = "/api/v5/account/balance"
	const method = http.MethodGet
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, nil)
	if err != nil {
		return nil, fmt.Errorf("okx balance request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OK-ACCESS-KEY", c.key)
	req.Header.Set("OK-ACCESS-SIGN", okxSign(timestamp, method, requestPath, "", c.secret))
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("okx balance request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("okx balance read failed: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("okx http %d: %s", resp.StatusCode, redact(data, c.key, c.secret, c.passphrase))
	}
	balances, err := parseBalances(data)
	if err != nil {
		return nil, err
	}
	return balances, nil
}

func okxSign(timestamp, method, requestPath, body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + strings.ToUpper(method) + requestPath + body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func parseBalances(data []byte) ([]Balance, error) {
	var raw struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Details []struct {
				CCY      string `json:"ccy"`
				AvailBal string `json:"availBal"`
				AvailEq  string `json:"availEq"`
				CashBal  string `json:"cashBal"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("okx balance decode failed: %w", err)
	}
	if raw.Code != "0" {
		return nil, fmt.Errorf("okx api code %s: %s", raw.Code, raw.Msg)
	}
	out := []Balance{}
	for _, account := range raw.Data {
		for _, detail := range account.Details {
			free := firstParseFloat(detail.AvailBal, detail.AvailEq, detail.CashBal)
			if detail.CCY == "" {
				continue
			}
			out = append(out, Balance{Asset: strings.ToUpper(detail.CCY), Free: free})
		}
	}
	return out, nil
}

func parseInstrumentFilters(data []byte) ([]InstrumentFilter, error) {
	var raw struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID      string `json:"instId"`
			TickSize    string `json:"tickSz"`
			LotSize     string `json:"lotSz"`
			MinSize     string `json:"minSz"`
			MinNotional string `json:"minNotional"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("okx instruments decode failed: %w", err)
	}
	if raw.Code != "0" {
		return nil, fmt.Errorf("okx instruments code %s: %s", raw.Code, raw.Msg)
	}
	out := []InstrumentFilter{}
	for _, item := range raw.Data {
		if item.InstID == "" {
			continue
		}
		out = append(out, InstrumentFilter{
			Symbol:      InternalSymbol(item.InstID),
			InstID:      item.InstID,
			MinNotional: firstParseFloat(item.MinNotional),
			MinSize:     firstParseFloat(item.MinSize),
			TickSize:    firstParseFloat(item.TickSize),
			StepSize:    firstParseFloat(item.LotSize),
		})
	}
	return out, nil
}

func OKXInstID(symbol string) string {
	s := strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
	if strings.HasSuffix(s, "USDT") && len(s) > 4 {
		return strings.TrimSuffix(s, "USDT") + "-USDT"
	}
	return symbol
}

func InternalSymbol(instID string) string {
	return strings.ToUpper(strings.ReplaceAll(instID, "-", ""))
}

func firstParseFloat(values ...string) float64 {
	for _, v := range values {
		if v == "" {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return 0
}

func parseOrderResult(data []byte, instID string) (OrderResult, error) {
	var raw struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrderID       string `json:"ordId"`
			ClientOrderID string `json:"clOrdId"`
			SCode         string `json:"sCode"`
			SMsg          string `json:"sMsg"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return OrderResult{}, fmt.Errorf("okx order decode failed: %w", err)
	}
	if raw.Code != "0" {
		return OrderResult{InstID: instID, Code: raw.Code, Message: raw.Msg}, fmt.Errorf("okx order code %s: %s", raw.Code, raw.Msg)
	}
	if len(raw.Data) == 0 {
		return OrderResult{InstID: instID}, fmt.Errorf("okx order returned no data")
	}
	item := raw.Data[0]
	result := OrderResult{InstID: instID, OrderID: item.OrderID, ClientOrderID: item.ClientOrderID, Code: item.SCode, Message: item.SMsg, Submitted: item.SCode == "0"}
	if !result.Submitted {
		return result, fmt.Errorf("okx order rejected code %s: %s", item.SCode, item.SMsg)
	}
	return result, nil
}

func parseCancelOrderResult(data []byte, instID string) (CancelOrderResult, error) {
	var raw struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			OrderID       string `json:"ordId"`
			ClientOrderID string `json:"clOrdId"`
			SCode         string `json:"sCode"`
			SMsg          string `json:"sMsg"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return CancelOrderResult{}, fmt.Errorf("okx cancel decode failed: %w", err)
	}
	if raw.Code != "0" {
		return CancelOrderResult{InstID: instID, Code: raw.Code, Message: raw.Msg}, fmt.Errorf("okx cancel code %s: %s", raw.Code, raw.Msg)
	}
	if len(raw.Data) == 0 {
		return CancelOrderResult{InstID: instID}, fmt.Errorf("okx cancel returned no data")
	}
	item := raw.Data[0]
	result := CancelOrderResult{InstID: instID, OrderID: item.OrderID, ClientOrderID: item.ClientOrderID, Code: item.SCode, Message: item.SMsg, Canceled: item.SCode == "0"}
	if !result.Canceled {
		return result, fmt.Errorf("okx cancel rejected code %s: %s", item.SCode, item.SMsg)
	}
	return result, nil
}

func normalizeOKXUnixTime(v int64) int64 {
	// OKX REST fields such as uTime are milliseconds since Unix epoch.
	// Local storage uses Unix seconds for *_at columns.
	if v > 9999999999 {
		return v / 1000
	}
	return v
}

func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func redact(data []byte, secrets ...string) string {
	out := string(bytes.TrimSpace(data))
	for _, secret := range secrets {
		if secret != "" {
			out = strings.ReplaceAll(out, secret, "<REDACTED>")
		}
	}
	if len(out) > 500 {
		out = out[:500] + "..."
	}
	return out
}

func (c *OKXClient) signedGet(ctx context.Context, requestPath string) ([]byte, error) {
	const method = http.MethodGet
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, nil)
	if err != nil {
		return nil, fmt.Errorf("okx signed get request creation: %w", err)
	}
	req.Header.Set("OK-ACCESS-KEY", c.key)
	req.Header.Set("OK-ACCESS-SIGN", okxSign(timestamp, method, requestPath, "", c.secret))
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", c.passphrase)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("okx signed get failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("okx signed get read failed: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("okx http %d: %s", resp.StatusCode, redact(data, c.key, c.secret, c.passphrase))
	}
	return data, nil
}

func (c *OKXClient) OrderStatus(ctx context.Context, instID, orderID, clientOrderID string) (OrderStatus, error) {
	var requestPath string
	if instID == "" {
		return OrderStatus{}, fmt.Errorf("instID required")
	}
	if orderID != "" {
		requestPath = fmt.Sprintf("/api/v5/trade/order?instId=%s&ordId=%s", url.QueryEscape(instID), url.QueryEscape(orderID))
	} else if clientOrderID != "" {
		requestPath = fmt.Sprintf("/api/v5/trade/order?instId=%s&clOrdId=%s", url.QueryEscape(instID), url.QueryEscape(clientOrderID))
	} else {
		return OrderStatus{}, fmt.Errorf("orderID or clientOrderID required")
	}

	data, err := c.signedGet(ctx, requestPath)
	if err != nil {
		return OrderStatus{}, err
	}

	statuses, err := parseOKXOrderStatus(data)
	if err != nil {
		return OrderStatus{}, err
	}
	if len(statuses) == 0 {
		return OrderStatus{}, fmt.Errorf("no order status data returned")
	}
	return statuses[0], nil
}

func (c *OKXClient) PendingOrders(ctx context.Context, instID string) ([]OrderStatus, error) {
	requestPath := "/api/v5/trade/orders-pending?instType=SPOT"
	if instID != "" {
		requestPath += "&instId=" + url.QueryEscape(instID)
	}
	data, err := c.signedGet(ctx, requestPath)
	if err != nil {
		return nil, err
	}
	return parseOKXOrderStatus(data)
}

func parseOKXOrderStatus(data []byte) ([]OrderStatus, error) {
	var raw struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID    string `json:"instId"`
			OrderID   string `json:"ordId"`
			ClOrdID   string `json:"clOrdId"`
			State     string `json:"state"`
			Side      string `json:"side"`
			OrdType   string `json:"ordType"`
			Px        string `json:"px"`
			Sz        string `json:"sz"`
			AccFillSz string `json:"accFillSz"`
			AvgPx     string `json:"avgPx"`
			Fee       string `json:"fee"`
			FeeCcy    string `json:"feeCcy"`
			UTime     string `json:"uTime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("okx decode order status failed: %w", err)
	}
	if raw.Code != "0" {
		return nil, fmt.Errorf("okx code %s: %s", raw.Code, raw.Msg)
	}
	out := []OrderStatus{}
	for _, item := range raw.Data {
		uTime, _ := strconv.ParseInt(item.UTime, 10, 64)
		uTime = normalizeOKXUnixTime(uTime)

		status := StatusUnknownNeedsManualCheck
		switch strings.ToLower(item.State) {
		case "live":
			status = StatusLiveOpen
		case "partially_filled":
			status = StatusPartiallyFilled
		case "filled":
			status = StatusFilled
		case "canceled":
			status = StatusCanceled
		case "rejected":
			status = StatusRejected
		}

		out = append(out, OrderStatus{
			InstID:            item.InstID,
			OrderID:           item.OrderID,
			ClientOrderID:     item.ClOrdID,
			State:             item.State,
			Status:            status,
			Side:              item.Side,
			OrderType:         item.OrdType,
			Price:             firstParseFloat(item.Px),
			Quantity:          firstParseFloat(item.Sz),
			FilledQuantity:    firstParseFloat(item.AccFillSz),
			AvgPrice:          firstParseFloat(item.AvgPx),
			AccumulatedFillSz: firstParseFloat(item.AccFillSz),
			Fee:               firstParseFloat(item.Fee),
			FeeCurrency:       strings.ToUpper(item.FeeCcy),
			UpdatedAt:         uTime,
		})
	}
	return out, nil
}
