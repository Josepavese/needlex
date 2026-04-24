package analytics

const (
	TokenEstimateMethod = "chars_per_token_estimate"
	CharsPerToken       = 4.0
)

type CostSavingsUSD struct {
	At1USDPerMillionTokens  float64 `json:"at_1_usd_per_million_tokens"`
	At5USDPerMillionTokens  float64 `json:"at_5_usd_per_million_tokens"`
	At10USDPerMillionTokens float64 `json:"at_10_usd_per_million_tokens"`
}

func TokenEstimateFromChars(chars int64) int64 {
	if chars <= 0 {
		return 0
	}
	return int64(float64(chars)/CharsPerToken + 0.5)
}

func CostSavingsFromTokens(tokens int64) CostSavingsUSD {
	return CostSavingsUSD{
		At1USDPerMillionTokens:  costAt(tokens, 1),
		At5USDPerMillionTokens:  costAt(tokens, 5),
		At10USDPerMillionTokens: costAt(tokens, 10),
	}
}

func costAt(tokens int64, usdPerMillion float64) float64 {
	if tokens <= 0 || usdPerMillion <= 0 {
		return 0
	}
	return float64(tokens) * usdPerMillion / 1_000_000
}
