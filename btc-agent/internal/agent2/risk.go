package agent2

func RewardRisk(entry, target, invalidation float64) float64 {
	if entry <= invalidation {
		return 0
	}
	return (target - entry) / (entry - invalidation)
}
