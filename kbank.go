package kbank

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/publicsuffix"
)

// KBank type
type KBank struct {
	username  string
	password  string
	accountNo string
	accountID string
	client    *http.Client
}

const (
	// OnlineEndPoint - kbank online endpoint
	OnlineEndPoint = "https://online.kasikornbankgroup.com/K-Online"

	// EBankEndPoint - kbank ebank endpoint
	EBankEndPoint = "https://ebank.kasikornbankgroup.com"
)

// New - returns new instance
func New(username, password, accountNo string) *KBank {
	jar, _ := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	return &KBank{
		username:  username,
		password:  password,
		accountNo: accountNo,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}
}

var (
	tokenIDPattern  = regexp.MustCompile(`<input type="hidden" name="tokenId" id="tokenId" value="([0-9]+)"/>`)
	txtParamPattern = regexp.MustCompile(` <input type="hidden" name="txtParam" value="([a-z0-9]+)" />`)
)

// Login ...
func (k *KBank) Login() error {
	u := OnlineEndPoint + "/login.do"
	preResponse, err := k.request("GET", u, nil)
	if err != nil {
		return err
	}
	tokenID := findString(tokenIDPattern.FindStringSubmatch(string(preResponse)), 1)

	data := url.Values{}
	data.Add("tokenId", tokenID)
	data.Add("userName", k.username)
	data.Add("password", k.password)
	data.Add("cmd", "authenticate")
	data.Add("locale", "en")
	data.Add("custType", "")
	data.Add("app", "0")
	_, err = k.request("POST", u, data)
	if err != nil {
		return err
	}
	if !k.CheckSession() {
		return fmt.Errorf("CheckSession: return false")
	}

	preResponse, err = k.request("GET", OnlineEndPoint+"/ib/redirectToIB.jsp", nil)
	if err != nil {
		return err
	}

	data = url.Values{}
	data.Add("txtParam", findString(txtParamPattern.FindStringSubmatch(string(preResponse)), 1))
	_, err = k.request("POST", EBankEndPoint+"/retail/security/Welcome.do", data)
	if err != nil {
		return err
	}
	return nil
}

// CheckSession - returns session is logged in
func (k *KBank) CheckSession() bool {
	res, err := k.request("POST", OnlineEndPoint+"/checkSession.jsp", nil)
	if err != nil {
		return false
	}

	return strings.Index(string(res), "<response><result>true</result></response>") != -1
}

// Transaction type
type Transaction struct {
	Time          time.Time
	Amount        float64
	FromAccountNo string
	Detail        string
}

var (
	tablibTokenPattern = regexp.MustCompile(`<input type="hidden" name="org\.apache\.struts\.taglib\.html\.TOKEN" value="([0-9.]+)">`)
	timePattern        = regexp.MustCompile(`([0-9]+)`)
)

// GetTransactions - returns today transactions
func (k *KBank) GetTransactions() ([]Transaction, error) {
	u := EBankEndPoint + "/retail/cashmanagement/TodayAccountStatementInquiry.do"
	preResponse, err := k.request("GET", u, nil)
	if err != nil {
		return nil, err
	}
	taglibToken := findString(tablibTokenPattern.FindStringSubmatch(string(preResponse)), 1)

	accountIDPattern := regexp.MustCompile(fmt.Sprintf(`<option value="([0-9]+)">%s-%s-%s-%s </option>`, k.accountNo[0:3], k.accountNo[3:4], k.accountNo[4:9], k.accountNo[9:10]))
	accountID := findString(accountIDPattern.FindStringSubmatch(string(preResponse)), 1)

	data := url.Values{}
	data.Add("org.apache.struts.taglib.html.TOKEN", taglibToken)
	data.Add("captcha_check", "null")
	data.Add("acctId", accountID)
	data.Add("action", "detail")
	data.Add("st", "0")
	res, err := k.request("POST", u, data)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(res)))
	if err != nil {
		return nil, err
	}

	out := make([]Transaction, 0)
	loc, _ := time.LoadLocation("Asia/Bangkok")
	doc.Find("#trans_detail").Find("tbody tr").Each(func(i int, tr *goquery.Selection) {
		timeMatches := timePattern.FindAllStringSubmatch(tr.Find("td:nth-child(1)").Text(), -1)
		if len(timeMatches) != 6 {
			return
		}

		timeTxt := fmt.Sprintf("%s/%s/%s %s:%s:%s", timeMatches[0][0], timeMatches[1][0], timeMatches[2][0], timeMatches[3][0], timeMatches[4][0], timeMatches[5][0])
		t, err := time.ParseInLocation("02/01/06 15:04:05", timeTxt, loc)
		if err != nil {
			return
		}

		amount, _ := strconv.ParseFloat(strings.ReplaceAll(tr.Find("td:nth-child(5)").Text(), ",", ""), 64)
		out = append(out, Transaction{
			Time:          t,
			Amount:        amount,
			FromAccountNo: strings.ReplaceAll(tr.Find("td:nth-child(6)").Text(), "-", ""),
			Detail:        tr.Find("td:nth-child(7)").Text(),
		})
	})
	return out, nil
}

func (k *KBank) request(method, u string, data url.Values) ([]byte, error) {
	if data == nil {
		data = url.Values{}
	}
	req, _ := http.NewRequest(method, u, strings.NewReader(data.Encode()))
	if method == "POST" {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := k.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	return body, nil
}

func findString(in []string, index int) string {
	if len(in) > index-1 {
		return in[index]
	}
	return ""
}
