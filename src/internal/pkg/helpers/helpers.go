package helpers

import "math"

func RoundToTwoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
