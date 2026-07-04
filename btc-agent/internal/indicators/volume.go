package indicators

func VolumeMA(volumes []float64, period int) []float64 {
	out := make([]float64, len(volumes))
	if period <= 0 {
		return out
	}
	sum := 0.0
	for i, volume := range volumes {
		sum += volume
		if i >= period {
			sum -= volumes[i-period]
		}
		if i >= period-1 {
			out[i] = sum / float64(period)
		}
	}
	return out
}
