package main

import (
	"bufio"
	"fmt"
	"github.com/sangx2/upbit"
	"github.com/sangx2/upbit/model/exchange"
	"github.com/sangx2/upbit/model/quotation"
	"github.com/slack-go/slack"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

// recentFall can be removed
type coin struct {
	name         string
	holdings     bool
	volume       float64
	avgprice     float64
	recentFall   bool
	tradable     bool
	tradableTime time.Time
	orderTime    time.Time
	uuid         string
}

func (c *coin) EncounterFalling(t time.Time) {
	c.recentFall = true
	c.tradableTime = t.Add(time.Hour * 24)
	c.tradable = false
}

func (c *coin) ResetFalling() {
	c.recentFall = false
}

func (c *coin) MakeTradable() {
	c.recentFall = false
	c.tradable = true
}

func (c *coin) GetRecentFall(u *upbit.Upbit) {
	time.Sleep(100000000)
	candles, _, err := u.GetMinuteCandles(c.name, "", "200", "10")
	for err != nil {
		time.Sleep(200000000)
		candles, _, err = u.GetMinuteCandles(c.name, "", "200", "10")
	}
	// check recent price falling
	recentClose := make([]float64, 200)
	recentTime := make([]string, 200)

	for i2, candle := range candles {
		recentClose[i2] = candle.TradePrice
		recentTime[i2] = candle.CandleDateTimeKST
	}
	var sum float64
	// from the past
	for i := 199; i > 179; i-- { // 180~ 199
		sum += recentClose[i]
	}
	recentAvg := make([]float64, 200)
	for i := 180; i > 0; i-- { // 180~199, 179~198
		recentAvg[i] = sum / 20
		if recentAvg[i]*0.97 > recentClose[i] {
			t := ChangeStringToTime(recentTime[i])
			c.EncounterFalling(t)
			break
		}
		sum -= recentClose[i+19]
		sum += recentClose[i-1]
	}
}

func GetWallet(u *upbit.Upbit, coins map[string]*coin) {
	var err error
	account, _, err := u.GetAccounts()
	if err != nil {
		log.Fatal("GetWallet", err)
	}
	for _, a := range account {
		if a.Currency != "KRW" {
			name := "KRW-" + a.Currency
			coins[name].holdings = true
			coins[name].volume, err = strconv.ParseFloat(a.Balance, 64)
			coins[name].tradable = false
			coins[name].tradableTime = time.Now().Add(time.Minute * 10)
			coins[name].avgprice, _ = strconv.ParseFloat(a.AvgBuyPrice, 64)
			if err != nil {
				fmt.Println("Err in GetWallet : ", err)
			}
		}
	}
}

func GetAvgBuyPrice(u *upbit.Upbit, coins map[string]*coin) {
	var err error
	account, _, err := u.GetAccounts()
	if err != nil {
		log.Fatal("GetAvgBuyPrice", err)
	}
	for _, a := range account {
		if a.Currency != "KRW" {
			name := "KRW-" + a.Currency
			coins[name].volume, err = strconv.ParseFloat(a.Balance, 64)
			coins[name].avgprice, _ = strconv.ParseFloat(a.AvgBuyPrice, 64)
			if err != nil {
				fmt.Println("Err in GetAvgBuyPrice : ", err)
			}
		}
	}
}

func GetKRWMarkets(u *upbit.Upbit) []*quotation.Market {
	all, _, err := u.GetMarkets()
	var krw []*quotation.Market
	if err == nil {
		for _, market := range all {
			if market.Market[:3] == "KRW" {
				krw = append(krw, market)
			}
		}
	} else {
		fmt.Println("Err in GetKRWMarkets : ", err)
	}
	return krw
}

func (c *coin) CheckTradablility(u *upbit.Upbit) {
	if c.tradableTime.Before(time.Now()) {
		c.tradable = true
		c.GetRecentFall(u)
	}
}

func (c *coin) CheckCoinStatus(u *upbit.Upbit) (string, float64) {
	c.CheckTradablility(u)
	time.Sleep(200000000)
	candle, _, err := u.GetMinuteCandles(c.name, "", "20", "10")
	num := len(candle)
	if num == 20 {
		if err != nil {
			fmt.Println("Err in CheckCoinStatus : ", err)
		}
		var sum float64
		for i := 0; i < num; i++ {
			sum += candle[i].TradePrice
		}
		currentPrice := candle[0].TradePrice
		openPrice := candle[0].OpeningPrice
		if c.tradable {
			if currentPrice >= sum/20*1.03 && currentPrice <= sum/20*1.05 && openPrice <= sum/20*1.03 {
				return "buy", currentPrice
			}
		} else if c.holdings {
			if currentPrice <= sum/20*0.97 || currentPrice <= c.avgprice*0.95 {
				return "sell", currentPrice
			}
		}
	}

	return "keep", -1
}

func (c *coin) BuyOrder(u *upbit.Upbit, price float64, amount float64) {
	v := amount / price
	volume := strconv.FormatFloat(v, 'f', -1, 64)
	p := strconv.FormatFloat(price, 'f', -1, 64)
	order, _, err := u.PurchaseOrder(c.name, volume, p, exchange.ORDER_TYPE_LIMIT, "")
	if err != nil {
		log.Fatal("BuyOrder", err)
	}
	c.volume += v
	c.uuid = order.UUID
	c.tradable = false
	c.orderTime = time.Now()
	c.tradableTime = c.orderTime.Add(time.Hour)
	time.Sleep(200000000)
}

func (c *coin) SellOrder(u *upbit.Upbit, amount float64) {
	v := c.volume * amount
	volume := strconv.FormatFloat(v, 'f', -1, 64)
	u.SellOrder(c.name, volume, "", exchange.ORDER_TYPE_MARKET, "")
	c.volume -= v
	c.EncounterFalling(time.Now())
	time.Sleep(200000000)
}

func (c *coin) CheckOrderResult(u *upbit.Upbit) {
	order, _, err := u.GetOrder(c.uuid, "")
	if err != nil {
		log.Fatal("CheckOrderResult", err)
	}
	if order.State == exchange.ORDER_STATE_WAIT && time.Now().After(c.orderTime.Add(time.Minute*4)) {
		u.CancelOrder(c.uuid, "")
		c.tradable = true
		c.tradableTime = time.Now()
		v, _ := strconv.ParseFloat(order.Volume, 64)
		c.volume -= v
	}
	time.Sleep(200000000)
}

func TradeCoin(u *upbit.Upbit, coins []*coin) {
	for _, c := range coins {
		action, price := c.CheckCoinStatus(u)
		if action == "buy" {
			c.BuyOrder(u, price, 6000)
		} else if action == "sell" {
			c.SellOrder(u, 1)
		} else if time.Now().Before(c.orderTime.Add(time.Minute * 5)) {
			c.CheckOrderResult(u)
		}
		//fmt.Println(c.name, action)
	}
}

func ChangeStringToTime(s string) time.Time {
	t, _ := time.Parse("2006-01-02T15:04:05", s)
	return t
}

func main() {
	//reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	fmt.Fprintln(writer, "start!!!")
	f, err := os.Open("key")
	if err != nil {
		fmt.Fprintln(writer, err)
	}
	scanner := bufio.NewScanner(f)
	scanner.Scan()
	access := scanner.Text()
	scanner.Scan()
	secret := scanner.Text()
	f.Close()
	// get my upbit account
	u := upbit.NewUpbit(access, secret)

	markets := GetKRWMarkets(u)
	coins := make(map[string]*coin)
	coinGroup := make(map[int][]*coin)
	// when execute first
	count := 0
	for _, market := range markets {
		name := market.Market
		coins[name] = &coin{name: name, holdings: false, volume: 0, recentFall: false, tradable: true}
		coins[name].GetRecentFall(u)
		//coinGroup[count][name] = coins[name]
		coinGroup[count] = append(coinGroup[count], coins[name])
		count += 1
		count %= 5
	}

	// check coins I have
	GetWallet(u, coins)
	//for _, c := range coins {
	//	fmt.Fprintln(writer, c)
	//}
	//fmt.Fprintln(writer, "!!!!!")

	f, err = os.Open("slack")
	if err != nil {
		fmt.Fprintln(writer, err)
	}
	scanner = bufio.NewScanner(f)
	scanner.Scan()
	token := scanner.Text()
	api := slack.New(token)

	text := "trading start"
	attachment := slack.Attachment{
		Text: text,
	}
	api.PostMessage("coin", slack.MsgOptionText("", false),
		slack.MsgOptionAttachments(attachment))

	var wait sync.WaitGroup
	sent := false
	// find coins to buy or sell
	for {
		wait.Add(5)
		go func() {
			TradeCoin(u, coinGroup[0])
			wait.Done()
		}()
		go func() {
			TradeCoin(u, coinGroup[1])
			wait.Done()
		}()
		go func() {
			TradeCoin(u, coinGroup[2])
			wait.Done()
		}()
		go func() {
			TradeCoin(u, coinGroup[3])
			wait.Done()
		}()
		go func() {
			TradeCoin(u, coinGroup[4])
			wait.Done()
		}()
		now := time.Now()
		if now.Minute()%30 == 0 && sent == false {
			GetAvgBuyPrice(u, coins)
			account, _, _ := u.GetAccounts()
			for _, a := range account {
				text := "avg buy price : " + a.AvgBuyPrice + ", total : " + a.Balance
				attachment := slack.Attachment{
					Title: a.Currency,
					Text:  text,
				}
				api.PostMessage("coin", slack.MsgOptionText("", false),
					slack.MsgOptionAttachments(attachment))
			}
			sent = true
		} else if now.Minute()%14 == 0 {
			sent = false
		}
		wait.Wait()
	}

}
