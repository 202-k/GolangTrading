package main

import (
	"bufio"
	"fmt"
	"github.com/sangx2/upbit"
	"github.com/sangx2/upbit/model/quotation"
	"os"
	"time"
)

// recentFall can be removed
type coin struct {
	name         string
	recentFall   bool
	tradable     bool
	tradabletime time.Time
	buyTime      time.Time
}

func (c *coin) EncounterFalling(t string) {
	c.recentFall = true
	changedTime := ChangeStringToTime(t)
	c.tradabletime = changedTime.Add(time.Hour * 24)
	c.tradable = false
}

func (c *coin) ResetFalling() {
	c.recentFall = false
}

func (c *coin) MakeTradable() {
	c.recentFall = false
	c.tradable = true
}

func (c *coin) GetRecentFall(u upbit.Upbit) {
	time.Sleep(100000000)
	candles, _, err := u.GetMinuteCandles(c.name, "", "200", "10")
	for err != nil {
		time.Sleep(100000000)
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
			c.EncounterFalling(recentTime[i])
			break
		}
		sum -= recentClose[i+19]
		sum += recentClose[i-1]
	}
}

func ChangeStringToTime(s string) time.Time {
	t, _ := time.Parse("2006-01-02T15:04:05", s)
	return t
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
	}
	return krw
}

func main() {
	//reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	f, err := os.Open("key")
	fmt.Fprintln(writer, err)
	scanner := bufio.NewScanner(f)
	scanner.Scan()
	access := scanner.Text()
	scanner.Scan()
	secret := scanner.Text()
	// get my upbit account
	u := upbit.NewUpbit(access, secret)
	// check coins I have
	account, _, _ := u.GetAccounts()
	for _, a := range account {
		fmt.Fprintln(writer, a)
	}
	markets := GetKRWMarkets(u)
	coins := make(map[string]*coin)
	// when execute first
	for _, market := range markets {
		name := market.Market
		coins[name] = &coin{name: name, recentFall: false, tradable: true}
		coins[name].GetRecentFall(*u)
	}

	//for s, c := range coins {
	//	fmt.Fprintf(writer, "%s : %v\n", s, c)
	//}

	// change down here with function with go routine
	// look for the rising coins
	//for {
	//	now := time.Now()
	//	for _, c := range coins {
	//		// check tradability
	//		if c.tradable == false { // found fall or bought in near time
	//			// check tradable time
	//			if now.After(c.tradabletime) {
	//				c.MakeTradable()
	//			}
	//		}
	//		// find buy strategy
	//
	//	}
	//}

}
