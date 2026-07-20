package config

import (
	"math"
	"testing"
)

func FuzzConfigValidation(f *testing.F) {
	for _, v := range []float64{-1, 0, .5, 1, math.Inf(1), math.NaN()} {
		f.Add(v, v)
	}
	f.Fuzz(func(t *testing.T, reserve, layer float64) {
		c := validTestConfig()
		c.Portfolio.ReserveCashRatio = reserve
		c.Execution.LayerDistribution = []float64{layer}
		err := c.Validate()
		valid := finiteConfigFloat(reserve) && reserve >= 0 && reserve < 1 && finiteConfigFloat(layer) && layer > 0 && layer >= .999 && layer <= 1.001
		if valid && err != nil {
			t.Fatalf("valid rejected: %v", err)
		}
		if !valid && err == nil {
			t.Fatalf("invalid accepted reserve=%v layer=%v", reserve, layer)
		}
	})
}
