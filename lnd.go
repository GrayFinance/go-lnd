package lnd

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/tidwall/gjson"
)

type Config struct {
	Host         string
	TlsCertData  []byte
	MacaroonData string
}

type Lnd struct {
	*Config
}

func Connect(host string, tls_cert_path string, macaroon_path string) *Lnd {
	tls_cert, err := ioutil.ReadFile(tls_cert_path)
	if err != nil {
		log.Fatal(err)
	}

	macaroon, err := ioutil.ReadFile(macaroon_path)
	if err != nil {
		log.Fatal(err)
	}

	lnd := &Lnd{
		Config: &Config{
			Host:         host,
			TlsCertData:  tls_cert,
			MacaroonData: hex.EncodeToString(macaroon),
		},
	}
	if _, err := lnd.GetInfo(); err != nil {
		log.Fatal(err)
	}
	return lnd
}

func (l Lnd) CallMake(method string, path string, params map[string]interface{}) (*http.Response, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, l.Host+"/"+path, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Grpc-Metadata-macaroon", l.MacaroonData)

	tls_cert := x509.NewCertPool()
	tls_cert.AppendCertsFromPEM(l.TlsCertData)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: tls_cert,
			},
		},
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return res, err
}

func (l Lnd) CallJSON(method string, path string, params map[string]interface{}) (gjson.Result, error) {
	res, err := l.CallMake(method, path, params)
	if err != nil {
		return gjson.Result{}, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return gjson.Result{}, err
	}

	rjson := gjson.ParseBytes(body)
	if rjson.String() == "0" {
		err := fmt.Errorf(string(body))
		return gjson.Result{}, err
	}

	if rjson.Get("error").String() != "" {
		err := fmt.Errorf(rjson.Get("error").Get("message").String())
		return gjson.Result{}, err
	}
	return rjson, nil
}

func (l Lnd) CallStream(method string, path string, params map[string]interface{}) (*bufio.Reader, error) {
	res, err := l.CallMake(method, path, params)
	if err != nil {
		return nil, err
	}
	return bufio.NewReader(res.Body), nil
}

func (l Lnd) CreateInvoice(value int, memo string) (gjson.Result, error) {
	data := map[string]interface{}{"value": value, "memo": memo}
	return l.CallJSON("POST", "v1/invoices", data)
}

func (l Lnd) ListInvoices() (gjson.Result, error) {
	return l.CallJSON("GET", "v1/invoices", nil)
}

func (l Lnd) PayInvoice(invoice string, fee_limit_msat float64) (gjson.Result, error) {
	data := map[string]interface{}{
		"timeout_seconds": 60,
		"payment_request": invoice,
		"fee_limit_msat":  fee_limit_msat,
	}
	return l.CallJSON("POST", "v2/router/send", data)
}

func (l Lnd) BalanceChannel() (gjson.Result, error) {
	return l.CallJSON("GET", "v1/balance/channels", nil)
}

func (l Lnd) DecodeInvoice(invoice string) (gjson.Result, error) {
	return l.CallJSON("GET", "v1/payreq/"+invoice, nil)
}

func (l Lnd) InvoicesSubscribe() (*bufio.Reader, error) {
	return l.CallStream("GET", "v1/invoices/subscribe", nil)
}

func (l Lnd) GetInfo() (gjson.Result, error) {
	return l.CallJSON("GET", "v1/getinfo", nil)
}
