package scanner

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/kol-tracker/pkg/config"
)

func weiToEth(weiStr string) float64 {
	if weiStr == "" || weiStr == "0" {
		return 0
	}
	wei, ok := new(big.Float).SetString(weiStr)
	if !ok {
		return 0
	}
	eth, _ := new(big.Float).Quo(wei, big.NewFloat(1e18)).Float64()
	return eth
}

func tokenValue(rawStr string, decimals int) float64 {
	if rawStr == "" || rawStr == "0" {
		return 0
	}
	val, ok := new(big.Float).SetString(rawStr)
	if !ok {
		return 0
	}
	divisor := new(big.Float).SetFloat64(math.Pow(10, float64(decimals)))
	result, _ := new(big.Float).Quo(val, divisor).Float64()
	return result
}

func parseUnixStr(s string) time.Time {
	ts, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func abbrev(addr string) string {
	if len(addr) > 12 {
		return addr[:6] + "..." + addr[len(addr)-4:]
	}
	return addr
}

func matchServiceLabel(label string) string {
	lower := strings.ToLower(label)
	for keyword, svcType := range config.ServiceLabels {
		if strings.Contains(lower, keyword) {
			return svcType
		}
	}
	if strings.Contains(lower, "exchange") || strings.Contains(lower, "cex") {
		return "cex"
	}
	return "unknown"
}

// unused suppresses import errors during development
func _unused() {
	_ = fmt.Sprintf
	_ = math.Abs
	_ = big.NewFloat
}
